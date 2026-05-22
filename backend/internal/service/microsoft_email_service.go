package service

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

const microsoftVerificationMessageLimit = 10

var microsoftVerificationCodePattern = regexp.MustCompile(`\b\d{6}\b`)

type MicrosoftEmailService struct {
	repo  MicrosoftEmailRepository
	graph MicrosoftGraphClient
}

func NewMicrosoftEmailService(repo MicrosoftEmailRepository, graph MicrosoftGraphClient) *MicrosoftEmailService {
	return &MicrosoftEmailService{repo: repo, graph: graph}
}

func (s *MicrosoftEmailService) ImportTXT(ctx context.Context, content string) (*MicrosoftEmailImportResult, error) {
	result := &MicrosoftEmailImportResult{}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	for idx, rawLine := range lines {
		lineNo := idx + 1
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		result.Total++
		parts := strings.Split(line, "----")
		if len(parts) != 4 {
			result.Failed++
			result.Errors = append(result.Errors, MicrosoftEmailImportError{Line: lineNo, Error: "invalid line format"})
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])
		clientID := strings.TrimSpace(parts[2])
		refreshToken := strings.TrimSpace(parts[3])
		if !isMicrosoftEmailAddress(email) || password == "" || clientID == "" || refreshToken == "" {
			result.Failed++
			result.Errors = append(result.Errors, MicrosoftEmailImportError{Line: lineNo, Email: email, Error: "invalid account fields"})
			continue
		}

		existing, err := s.repo.GetByEmail(ctx, email)
		if err == nil {
			updated, err := s.repo.UpdateCredentials(ctx, existing.ID, MicrosoftEmailCredentialUpdate{Password: password, ClientID: clientID, RefreshToken: refreshToken})
			if err != nil {
				return nil, err
			}
			result.Updated++
			result.Items = append(result.Items, MicrosoftEmailImportItem{Line: lineNo, Email: email, Action: "updated", Account: MaskMicrosoftEmailAccount(updated)})
			continue
		}
		if !errors.Is(err, ErrMicrosoftEmailNotFound) {
			return nil, err
		}

		created, err := s.repo.Create(ctx, &MicrosoftEmailAccount{Email: email, Password: password, ClientID: clientID, RefreshToken: refreshToken, Status: MicrosoftEmailStatusActive})
		if err != nil {
			return nil, err
		}
		result.Created++
		result.Items = append(result.Items, MicrosoftEmailImportItem{Line: lineNo, Email: email, Action: "created", Account: MaskMicrosoftEmailAccount(created)})
	}

	return result, nil
}

func (s *MicrosoftEmailService) Check(ctx context.Context, id int64) (*MicrosoftEmailCheckResult, error) {
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	checkedAt := time.Now()
	status := MicrosoftEmailStatusActive
	var lastErr *string
	_, err = s.graph.RefreshAccessToken(ctx, account.ClientID, account.RefreshToken)
	if err != nil {
		status = MicrosoftEmailStatusInvalid
		msg := sanitizeMicrosoftSecretError(err, account.Password, account.RefreshToken, account.ClientID)
		lastErr = &msg
	}

	if err := s.repo.UpdateCheckResult(ctx, id, status, checkedAt, lastErr); err != nil {
		return nil, err
	}

	return &MicrosoftEmailCheckResult{ID: account.ID, Email: account.Email, Status: status, CheckedAt: checkedAt, LastError: lastErr}, nil
}

func (s *MicrosoftEmailService) FetchCode(ctx context.Context, id int64) (*MicrosoftEmailFetchCodeResult, error) {
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	fetchedAt := time.Now()
	result := &MicrosoftEmailFetchCodeResult{Email: account.Email}
	accessToken, err := s.graph.RefreshAccessToken(ctx, account.ClientID, account.RefreshToken)
	if err != nil {
		msg := sanitizeMicrosoftSecretError(err, account.Password, account.RefreshToken, account.ClientID)
		result.Error = msg
		status := MicrosoftEmailStatusInvalid
		if err := s.repo.UpdateFetchResult(ctx, id, fetchedAt, &status, &msg); err != nil {
			return nil, err
		}
		return result, nil
	}

	messages, err := s.graph.ListRecentMessages(ctx, accessToken, microsoftVerificationMessageLimit)
	if err != nil {
		msg := sanitizeMicrosoftSecretError(err, account.Password, account.RefreshToken, account.ClientID, accessToken)
		result.Error = msg
		status := MicrosoftEmailStatusError
		if err := s.repo.UpdateFetchResult(ctx, id, fetchedAt, &status, &msg); err != nil {
			return nil, err
		}
		return result, nil
	}

	extracted := ExtractMicrosoftVerificationCode(messages)
	if extracted.Code == "" {
		msg := "code_not_found"
		result.Error = msg
		if err := s.repo.UpdateFetchResult(ctx, id, fetchedAt, nil, &msg); err != nil {
			return nil, err
		}
		return result, nil
	}

	result.Code = extracted.Code
	result.Source = extracted.Source
	result.Subject = extracted.Subject
	result.From = extracted.From
	result.ReceivedAt = extracted.ReceivedAt
	result.Snippet = extracted.Snippet
	status := MicrosoftEmailStatusActive
	if err := s.repo.UpdateFetchResult(ctx, id, fetchedAt, &status, nil); err != nil {
		return nil, err
	}
	return result, nil
}

func ExtractMicrosoftVerificationCode(messages []MicrosoftGraphMessage) MicrosoftEmailFetchCodeResult {
	if len(messages) == 0 {
		return MicrosoftEmailFetchCodeResult{}
	}

	sortedMessages := append([]MicrosoftGraphMessage(nil), messages...)
	sort.SliceStable(sortedMessages, func(i, j int) bool {
		return sortedMessages[i].ReceivedAt.After(sortedMessages[j].ReceivedAt)
	})

	var fallback *MicrosoftEmailFetchCodeResult
	for i := range sortedMessages {
		msg := sortedMessages[i]
		for _, part := range []struct {
			source string
			text   string
		}{
			{source: "subject", text: msg.Subject},
			{source: "body", text: strings.TrimSpace(msg.BodyPreview + " " + msg.BodyText)},
		} {
			code := microsoftVerificationCodePattern.FindString(part.text)
			if code == "" {
				continue
			}
			candidate := MicrosoftEmailFetchCodeResult{
				Code:       code,
				Source:     part.source,
				Subject:    msg.Subject,
				From:       msg.From,
				ReceivedAt: msg.ReceivedAt,
				Snippet:    msg.BodyPreview,
			}
			if isMicrosoftVerificationKeywordText(msg.Subject + " " + msg.BodyPreview + " " + msg.BodyText) {
				return candidate
			}
			if fallback == nil {
				fallback = &candidate
			}
		}
	}

	if fallback != nil {
		return *fallback
	}
	return MicrosoftEmailFetchCodeResult{}
}

func MaskMicrosoftEmailAccount(account *MicrosoftEmailAccount) *MicrosoftEmailAccount {
	if account == nil {
		return nil
	}
	masked := *account
	masked.Password = maskSecret(masked.Password)
	masked.ClientID = maskSecret(masked.ClientID)
	masked.RefreshToken = maskSecret(masked.RefreshToken)
	return &masked
}

func isMicrosoftEmailAddress(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || strings.ContainsAny(email, " \t\n\r") {
		return false
	}
	parts := strings.Split(email, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".")
}

func maskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}

func sanitizeMicrosoftSecretError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	for _, token := range []string{"refresh_token", "access_token", "client_secret", "password"} {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	return msg
}

func isMicrosoftVerificationKeywordText(text string) bool {
	text = strings.ToLower(text)
	keywords := []string{"verification", "verify", "code", "security", "login", "sign in", "microsoft"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
