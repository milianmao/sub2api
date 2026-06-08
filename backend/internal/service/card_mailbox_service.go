package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	maxCardMailboxResponseBytes = 2 << 20
	cardMailboxFetchTimeout     = 10 * time.Second
)

type cardMailboxHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type cardMailboxResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type netCardMailboxResolver struct{}

func (netCardMailboxResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

type CardMailboxService struct {
	repo       CardMailboxRepository
	httpClient cardMailboxHTTPClient
	resolver   cardMailboxResolver
}

func NewCardMailboxService(repo CardMailboxRepository, httpClient cardMailboxHTTPClient) *CardMailboxService {
	resolver := netCardMailboxResolver{}
	if httpClient == nil {
		httpClient = newCardMailboxHTTPClient(resolver)
	}
	return &CardMailboxService{repo: repo, httpClient: httpClient, resolver: resolver}
}

func (s *CardMailboxService) List(ctx context.Context, filter CardMailboxListFilter) ([]*CardMailbox, int, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrCardMailboxDependencyMissing
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	masked := make([]*CardMailbox, 0, len(items))
	for _, item := range items {
		masked = append(masked, MaskCardMailbox(item))
	}
	return masked, total, nil
}

func (s *CardMailboxService) ImportJSONL(ctx context.Context, r io.Reader) (*CardMailboxImportResult, error) {
	result := &CardMailboxImportResult{}
	if s == nil || s.repo == nil {
		return result, ErrCardMailboxDependencyMissing
	}
	if r == nil {
		return result, nil
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxCardMailboxResponseBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		input, err := parseCardMailboxJSONLLine([]byte(line))
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, CardMailboxImportError{Line: lineNumber, Message: redactCardMailboxSensitive(err.Error())})
			continue
		}

		if _, err := s.repo.UpsertByEmail(ctx, input); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, CardMailboxImportError{Line: lineNumber, Message: redactCardMailboxSensitive(err.Error())})
			continue
		}
		result.Imported++
	}
	if err := scanner.Err(); err != nil {
		result.Failed++
		result.Errors = append(result.Errors, CardMailboxImportError{Line: lineNumber + 1, Message: "failed to read import input"})
	}

	return result, nil
}

func (s *CardMailboxService) Delete(ctx context.Context, id int64) error {
	if s == nil || s.repo == nil {
		return ErrCardMailboxDependencyMissing
	}
	return s.repo.Delete(ctx, id)
}

func (s *CardMailboxService) ExportByIDs(ctx context.Context, ids []int64) ([]CardMailboxExportItem, error) {
	if s == nil || s.repo == nil {
		return nil, ErrCardMailboxDependencyMissing
	}
	if len(ids) == 0 {
		return []CardMailboxExportItem{}, nil
	}

	items, err := s.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	indexByID := make(map[int64]*CardMailbox, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		indexByID[item.ID] = item
	}

	result := make([]CardMailboxExportItem, 0, len(ids))
	for _, id := range ids {
		item, ok := indexByID[id]
		if !ok || item == nil {
			continue
		}
		result = append(result, CardMailboxExportItem{
			ID:            item.ID,
			Email:         item.Email,
			MailboxURL:    item.MailboxURL,
			RawJSON:       item.RawJSON,
			LastCode:      item.LastCode,
			LastStatus:    item.LastStatus,
			LastError:     item.LastError,
			LastFetchedAt: item.LastFetchedAt,
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	return result, nil
}

func (s *CardMailboxService) FetchCode(ctx context.Context, id int64) (*CardMailboxFetchResult, error) {
	if s != nil && s.resolver == nil {
		s.resolver = netCardMailboxResolver{}
	}
	if s == nil || s.repo == nil || s.httpClient == nil {
		return nil, ErrCardMailboxDependencyMissing
	}
	if id <= 0 {
		return nil, ErrCardMailboxInvalidInput
	}

	mailbox, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	fetchCtx, cancel := context.WithTimeout(ctx, cardMailboxFetchTimeout)
	defer cancel()

	fetchedAt := time.Now()
	fail := func(publicErr *infraerrors.ApplicationError, cause error) (*CardMailboxFetchResult, error) {
		message := redactCardMailboxSensitive(cause.Error())
		if message == "" {
			message = publicErr.Message
		}
		if err := s.repo.UpdateLatestResult(ctx, mailbox.ID, CardMailboxLatestResultInput{
			LastStatus:    CardMailboxFetchStatusFailed,
			LastError:     message,
			LastFetchedAt: &fetchedAt,
		}); err != nil {
			return nil, ErrCardMailboxRepositoryFailure.WithCause(fmt.Errorf("failed to persist mailbox fetch failure: %s", redactCardMailboxSensitive(err.Error())))
		}
		return nil, publicErr.WithCause(fmt.Errorf("%s", message))
	}

	if err := validateCardMailboxURL(fetchCtx, mailbox.MailboxURL, s.resolver); err != nil {
		return fail(ErrCardMailboxInvalidInput, err)
	}

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, mailbox.MailboxURL, nil)
	if err != nil {
		return fail(ErrCardMailboxInvalidInput, err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fail(ErrCardMailboxFetchFailed, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCardMailboxResponseBytes))
	if err != nil {
		return fail(ErrCardMailboxFetchFailed, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fail(ErrCardMailboxFetchFailed, fmt.Errorf("mailbox returned status %d", resp.StatusCode))
	}

	code := ExtractCardMailboxCode(body)
	if code == "" {
		return fail(ErrCardMailboxCodeNotFound, fmt.Errorf("verification code not found"))
	}
	metadata := ExtractCardMailboxFetchMetadata(body, code)

	if err := s.repo.UpdateLatestResult(ctx, mailbox.ID, CardMailboxLatestResultInput{
		LastCode:      code,
		LastStatus:    CardMailboxFetchStatusSuccess,
		LastFetchedAt: &fetchedAt,
	}); err != nil {
		return nil, ErrCardMailboxRepositoryFailure.WithCause(fmt.Errorf("update latest result failed"))
	}

	return &CardMailboxFetchResult{
		Email:      mailbox.Email,
		Code:       code,
		Status:     CardMailboxFetchStatusSuccess,
		FetchedAt:  fetchedAt,
		Source:     metadata.Source,
		Subject:    metadata.Subject,
		From:       metadata.From,
		ReceivedAt: metadata.ReceivedAt,
		Snippet:    metadata.Snippet,
	}, nil
}

type cardMailboxDialContext func(ctx context.Context, network, address string) (net.Conn, error)

func newCardMailboxHTTPClient(resolver cardMailboxResolver) *http.Client {
	return newCardMailboxHTTPClientWithDialer(resolver, (&net.Dialer{Timeout: cardMailboxFetchTimeout}).DialContext)
}

func newCardMailboxHTTPClientWithDialer(resolver cardMailboxResolver, dialContext cardMailboxDialContext) *http.Client {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		defaultTransport = &http.Transport{}
	}
	transport := defaultTransport.Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolveValidatedCardMailboxIPs(ctx, host, resolver)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("mailbox url host could not be resolved")
		}
		return dialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{
		Timeout:   cardMailboxFetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return validateCardMailboxParsedURL(req.Context(), req.URL, resolver)
		},
	}
}

func parseCardMailboxJSONLLine(line []byte) (CardMailboxUpsertInput, error) {
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return CardMailboxUpsertInput{}, fmt.Errorf("invalid json")
	}

	email := firstStringAlias(payload, "email", "mail", "address", "mailbox_email", "mailboxEmail")
	mailboxURL := firstStringAlias(payload, "mailbox_url", "mailboxUrl", "mailboxURL", "url", "mail_url", "mailUrl")
	email = strings.TrimSpace(strings.ToLower(email))
	mailboxURL = strings.TrimSpace(mailboxURL)
	if email == "" {
		return CardMailboxUpsertInput{}, fmt.Errorf("email is required")
	}
	if mailboxURL == "" {
		return CardMailboxUpsertInput{}, fmt.Errorf("mailbox url is required")
	}
	if err := validateCardMailboxURL(context.Background(), mailboxURL, nil); err != nil {
		return CardMailboxUpsertInput{}, err
	}
	return CardMailboxUpsertInput{Email: email, MailboxURL: mailboxURL, RawJSON: string(line)}, nil
}

func validateCardMailboxURL(ctx context.Context, rawURL string, resolver cardMailboxResolver) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed == nil {
		return fmt.Errorf("mailbox url is invalid")
	}
	return validateCardMailboxParsedURL(ctx, parsed, resolver)
}

func validateCardMailboxParsedURL(ctx context.Context, parsed *url.URL, resolver cardMailboxResolver) error {
	if parsed == nil {
		return fmt.Errorf("mailbox url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("mailbox url scheme is not allowed")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("mailbox url host is required")
	}
	return validateCardMailboxHostResolved(ctx, host, resolver)
}

func validateCardMailboxHostResolved(ctx context.Context, host string, resolver cardMailboxResolver) error {
	_, err := resolveValidatedCardMailboxIPs(ctx, host, resolver)
	return err
}

func resolveValidatedCardMailboxIPs(ctx context.Context, host string, resolver cardMailboxResolver) ([]net.IP, error) {
	host = strings.Trim(strings.ToLower(host), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, fmt.Errorf("mailbox url host is not allowed")
	}
	if strings.Contains(host, "metadata") || host == "metadata.google.internal" {
		return nil, fmt.Errorf("mailbox url host is not allowed")
	}
	literalIP := net.ParseIP(host)
	if literalIP != nil {
		if isBlockedCardMailboxIP(literalIP) {
			return nil, fmt.Errorf("mailbox url host is not allowed")
		}
		return []net.IP{literalIP}, nil
	}
	if resolver == nil {
		return nil, nil
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("mailbox url host could not be resolved")
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("mailbox url host could not be resolved")
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if isBlockedCardMailboxIP(addr.IP) {
			return nil, fmt.Errorf("mailbox url host is not allowed")
		}
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func isBlockedCardMailboxIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func firstStringAlias(payload map[string]any, aliases ...string) string {
	for _, alias := range aliases {
		if value, ok := payload[alias]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func ExtractCardMailboxCode(body []byte) string {
	text := normalizeCardMailboxResponse(body)
	if code := findKeywordNumericCardMailboxCode(text); code != "" {
		return code
	}
	if code := findNumericCardMailboxCode(text); code != "" {
		return code
	}
	return findKeywordAlphanumericCardMailboxCode(text)
}

type CardMailboxFetchMetadata struct {
	Source     string
	Subject    string
	From       string
	ReceivedAt time.Time
	Snippet    string
}

func ExtractCardMailboxFetchMetadata(body []byte, code string) CardMailboxFetchMetadata {
	metadata := CardMailboxFetchMetadata{Source: "body"}
	var payload any
	if json.Unmarshal(body, &payload) == nil {
		metadata.Subject = safeCardMailboxJSONString(payload, "subject", "Subject")
		metadata.From = safeCardMailboxJSONString(payload, "from", "sender", "from_email", "fromEmail")
		metadata.ReceivedAt = parseCardMailboxTime(safeCardMailboxJSONString(payload, "received_at", "receivedAt", "date", "created_at", "createdAt"))
		metadata.Snippet = safeCardMailboxJSONString(payload, "snippet", "body_preview", "bodyPreview", "preview")
		if metadata.Snippet == "" {
			metadata.Snippet = normalizeCardMailboxResponse(body)
		}
	} else {
		metadata.Snippet = normalizeCardMailboxResponse(body)
	}
	metadata.Subject = trimSafeCardMailboxMetadata(metadata.Subject, 200)
	metadata.From = trimSafeCardMailboxMetadata(metadata.From, 200)
	metadata.Snippet = trimSafeCardMailboxMetadata(metadata.Snippet, 300)
	if metadata.Subject != "" && strings.Contains(metadata.Subject, code) {
		metadata.Source = "subject"
	}
	return metadata
}

func normalizeCardMailboxResponse(body []byte) string {
	var payload any
	if json.Unmarshal(body, &payload) == nil {
		var b strings.Builder
		appendJSONStrings(&b, payload)
		if b.Len() > 0 {
			return html.UnescapeString(stripHTMLTags(b.String()))
		}
	}
	return html.UnescapeString(stripHTMLTags(string(body)))
}

func safeCardMailboxJSONString(value any, aliases ...string) string {
	for _, alias := range aliases {
		if found, ok := findCardMailboxJSONString(value, alias); ok {
			return found
		}
	}
	return ""
}

func findCardMailboxJSONString(value any, alias string) (string, bool) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if strings.EqualFold(key, alias) {
				if s, ok := item.(string); ok {
					return s, true
				}
			}
			if found, ok := findCardMailboxJSONString(item, alias); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range v {
			if found, ok := findCardMailboxJSONString(item, alias); ok {
				return found, true
			}
		}
	}
	return "", false
}

func parseCardMailboxTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func trimSafeCardMailboxMetadata(value string, limit int) string {
	value = redactCardMailboxSensitive(html.UnescapeString(stripHTMLTags(value)))
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func appendJSONStrings(b *strings.Builder, value any) {
	switch v := value.(type) {
	case string:
		_ = b.WriteByte(' ')
		_, _ = b.WriteString(v)
	case float64:
		_ = b.WriteByte(' ')
		_, _ = b.WriteString(fmt.Sprintf("%.0f", v))
	case json.Number:
		_ = b.WriteByte(' ')
		_, _ = b.WriteString(v.String())
	case bool:
		_ = b.WriteByte(' ')
		_, _ = b.WriteString(fmt.Sprintf("%t", v))
	case []any:
		for _, item := range v {
			appendJSONStrings(b, item)
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteByte(' ')
			b.WriteString(key)
			appendJSONStrings(b, v[key])
		}
	}
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTMLTags(text string) string {
	return htmlTagRe.ReplaceAllString(text, " ")
}

var numericCardMailboxCodeRe = regexp.MustCompile(`\b\d{4,8}\b`)

func findKeywordNumericCardMailboxCode(text string) string {
	keywordIndexes := keywordCardMailboxRe.FindAllStringIndex(text, -1)
	if len(keywordIndexes) == 0 {
		return ""
	}
	candidates := numericCardMailboxCodeRe.FindAllStringIndex(text, -1)
	for _, keywordIndex := range keywordIndexes {
		keywordEnd := keywordIndex[1]
		best := ""
		bestDistance := 81
		for _, candidateIndex := range candidates {
			distance := absInt(candidateIndex[0] - keywordEnd)
			if distance > 80 || distance >= bestDistance {
				continue
			}
			bestDistance = distance
			best = text[candidateIndex[0]:candidateIndex[1]]
		}
		if best != "" {
			return best
		}
	}
	return ""
}

func findNumericCardMailboxCode(text string) string {
	return numericCardMailboxCodeRe.FindString(text)
}

var (
	keywordCardMailboxRe       = regexp.MustCompile(`(?i)\b(?:code|verification|verify|token|otp|passcode)\b`)
	alphanumericCardMailboxRe  = regexp.MustCompile(`\b[A-Za-z0-9]{4,12}\b`)
	cardMailboxKeywordStopword = map[string]struct{}{
		"CODE": {}, "VERIFICATION": {}, "VERIFY": {}, "TOKEN": {}, "OTP": {}, "PASSCODE": {},
		"YOUR": {}, "THIS": {}, "THAT": {}, "WITH": {}, "FROM": {}, "MAIL": {}, "EMAIL": {},
	}
)

func findKeywordAlphanumericCardMailboxCode(text string) string {
	keywordIndexes := keywordCardMailboxRe.FindAllStringIndex(text, -1)
	if len(keywordIndexes) == 0 {
		return ""
	}
	candidates := alphanumericCardMailboxRe.FindAllStringIndex(text, -1)
	for _, keywordIndex := range keywordIndexes {
		keywordEnd := keywordIndex[1]
		sort.SliceStable(candidates, func(i, j int) bool {
			return absInt(candidates[i][0]-keywordEnd) < absInt(candidates[j][0]-keywordEnd)
		})
		for _, candidateIndex := range candidates {
			if absInt(candidateIndex[0]-keywordEnd) > 80 {
				continue
			}
			candidate := strings.ToUpper(text[candidateIndex[0]:candidateIndex[1]])
			if _, stop := cardMailboxKeywordStopword[candidate]; stop {
				continue
			}
			if hasLetter(candidate) && hasDigit(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

var sensitiveCardMailboxFields = []string{"refresh_token", "access_token", "mailbox_url", "password", "token"}

func RedactCardMailboxSensitive(message string) string {
	return redactCardMailboxSensitive(message)
}

func MaskCardMailbox(mailbox *CardMailbox) *CardMailbox {
	if mailbox == nil {
		return nil
	}
	masked := *mailbox
	masked.MailboxURL = maskSecret(masked.MailboxURL)
	return &masked
}

func redactCardMailboxSensitive(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	redacted := regexp.MustCompile(`https?://[^\s]+`).ReplaceAllString(message, "[REDACTED]")
	for _, field := range sensitiveCardMailboxFields {
		redacted = regexp.MustCompile(`(?i)(?:[?&;\s,{\[]|^)`+regexp.QuoteMeta(field)+`\s*[:=]\s*"?[^"\s,;&}\]]+"?`).ReplaceAllString(redacted, " [REDACTED]")
		redacted = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(field)+`\s*[:=]\s*"?[^"\s,;&}\]]+"?`).ReplaceAllString(redacted, "[REDACTED]")
		redacted = regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(field)+`\b`).ReplaceAllString(redacted, "[REDACTED]")
	}
	redacted = strings.Join(strings.Fields(redacted), " ")
	if redacted == "" {
		return "[REDACTED]"
	}
	return redacted
}
