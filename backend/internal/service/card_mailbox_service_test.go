package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type cardMailboxRepoStub struct {
	lists         []CardMailboxListFilter
	deletes       []int64
	upserts       []CardMailboxUpsertInput
	getByIDs      [][]int64
	latestUpdates []CardMailboxLatestResultInput
	byEmail       map[string]*CardMailbox
	upsertErr     error
	getErr        error
	getByIDsErr   error
	updateErr     error
}

func (s *cardMailboxRepoStub) List(_ context.Context, filter CardMailboxListFilter) ([]*CardMailbox, int, error) {
	s.lists = append(s.lists, filter)
	items := make([]*CardMailbox, 0, len(s.byEmail))
	for _, item := range s.byEmail {
		items = append(items, item)
	}
	return items, len(items), nil
}

func (s *cardMailboxRepoStub) UpsertByEmail(_ context.Context, input CardMailboxUpsertInput) (*CardMailbox, error) {
	if s.upsertErr != nil {
		return nil, s.upsertErr
	}
	s.upserts = append(s.upserts, input)
	if s.byEmail == nil {
		s.byEmail = make(map[string]*CardMailbox)
	}
	item, ok := s.byEmail[input.Email]
	if !ok {
		item = &CardMailbox{ID: int64(len(s.byEmail) + 1)}
		s.byEmail[input.Email] = item
	}
	item.Email = input.Email
	item.MailboxURL = input.MailboxURL
	item.RawJSON = input.RawJSON
	item.UpdatedAt = time.Unix(1776790000, 0)
	return item, nil
}

func (s *cardMailboxRepoStub) GetByID(_ context.Context, id int64) (*CardMailbox, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, item := range s.byEmail {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, ErrCardMailboxNotFound
}

func (s *cardMailboxRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*CardMailbox, error) {
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
	}
	copied := append([]int64(nil), ids...)
	s.getByIDs = append(s.getByIDs, copied)
	items := make([]*CardMailbox, 0, len(ids))
	for _, id := range ids {
		for _, item := range s.byEmail {
			if item.ID == id {
				items = append(items, item)
				break
			}
		}
	}
	return items, nil
}

func (s *cardMailboxRepoStub) Delete(_ context.Context, id int64) error {
	s.deletes = append(s.deletes, id)
	return nil
}

func (s *cardMailboxRepoStub) UpdateLatestResult(_ context.Context, id int64, input CardMailboxLatestResultInput) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.latestUpdates = append(s.latestUpdates, input)
	for _, item := range s.byEmail {
		if item.ID == id {
			item.LastCode = input.LastCode
			item.LastStatus = input.LastStatus
			item.LastError = input.LastError
			item.LastFetchedAt = input.LastFetchedAt
			break
		}
	}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type cardMailboxResolverStub struct {
	ips map[string][]net.IPAddr
	err error
}

func (s cardMailboxResolverStub) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if s.err != nil {
		return nil, s.err
	}
	if ips, ok := s.ips[host]; ok {
		return ips, nil
	}
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func TestCardMailboxImportJSONLParsesAliasesAndUpsertsByEmail(t *testing.T) {
	repo := &cardMailboxRepoStub{}
	svc := NewCardMailboxService(repo, nil)

	result, err := svc.ImportJSONL(context.Background(), strings.NewReader(strings.Join([]string{
		`{"email":"first@example.com","mailbox_url":"https://mail.example.com/first?token=secret"}`,
		`{"mail":"second@example.com","mailboxUrl":"https://mail.example.com/second"}`,
		`{"address":"third@example.com","url":"https://mail.example.com/third"}`,
		`{"email":"missing-url@example.com"}`,
	}, "\n")))

	require.NoError(t, err)
	require.Equal(t, 3, result.Imported)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Errors, 1)
	require.Contains(t, result.Errors[0].Message, "mailbox url is required")
	require.NotContains(t, result.Errors[0].Message, "secret")
	require.Len(t, repo.upserts, 3)
	require.Equal(t, CardMailboxUpsertInput{
		Email:      "first@example.com",
		MailboxURL: "https://mail.example.com/first?token=secret",
		RawJSON:    `{"email":"first@example.com","mailbox_url":"https://mail.example.com/first?token=secret"}`,
	}, repo.upserts[0])
	require.Equal(t, CardMailboxUpsertInput{
		Email:      "second@example.com",
		MailboxURL: "https://mail.example.com/second",
		RawJSON:    `{"mail":"second@example.com","mailboxUrl":"https://mail.example.com/second"}`,
	}, repo.upserts[1])
	require.Equal(t, CardMailboxUpsertInput{
		Email:      "third@example.com",
		MailboxURL: "https://mail.example.com/third",
		RawJSON:    `{"address":"third@example.com","url":"https://mail.example.com/third"}`,
	}, repo.upserts[2])
}

func TestCardMailboxExportSelectedReturnsFullEntries(t *testing.T) {
	repo := &cardMailboxRepoStub{
		byEmail: map[string]*CardMailbox{
			"a@example.com": {
				ID:            11,
				Email:         "a@example.com",
				MailboxURL:    "https://mail.example.com/a?token=secret",
				RawJSON:       `{"email":"a@example.com","mailbox_url":"https://mail.example.com/a?token=secret","password":"pw1"}`,
				LastCode:      "123456",
				LastStatus:    CardMailboxFetchStatusSuccess,
				LastError:     "",
				LastFetchedAt: cardMailboxPtrTime(time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)),
				CreatedAt:     time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC),
				UpdatedAt:     time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC),
			},
			"b@example.com": {
				ID:         12,
				Email:      "b@example.com",
				MailboxURL: "https://mail.example.com/b",
				RawJSON:    `{"email":"b@example.com","mailbox_url":"https://mail.example.com/b","custom":"x"}`,
				CreatedAt:  time.Date(2026, 5, 27, 11, 0, 0, 0, time.UTC),
				UpdatedAt:  time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := NewCardMailboxService(repo, nil)

	result, err := svc.ExportByIDs(context.Background(), []int64{12, 11})

	require.NoError(t, err)
	require.Equal(t, [][]int64{{12, 11}}, repo.getByIDs)
	require.Len(t, result, 2)
	require.Equal(t, int64(12), result[0].ID)
	require.Equal(t, `{"email":"b@example.com","mailbox_url":"https://mail.example.com/b","custom":"x"}`, result[0].RawJSON)
	require.Equal(t, "https://mail.example.com/b", result[0].MailboxURL)
	require.Equal(t, int64(11), result[1].ID)
	require.Equal(t, `{"email":"a@example.com","mailbox_url":"https://mail.example.com/a?token=secret","password":"pw1"}`, result[1].RawJSON)
	require.Equal(t, "123456", result[1].LastCode)
}

func cardMailboxPtrTime(v time.Time) *time.Time {
	return &v
}

func TestCardMailboxImportJSONLRedactsSensitiveParseErrors(t *testing.T) {
	repo := &cardMailboxRepoStub{}
	svc := NewCardMailboxService(repo, nil)

	result, err := svc.ImportJSONL(context.Background(), strings.NewReader(`{"email":"leak@example.com","mailbox_url":"https://mail.example.com/?access_token=super-secret"`))

	require.NoError(t, err)
	require.Equal(t, 0, result.Imported)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Errors, 1)
	require.NotContains(t, result.Errors[0].Message, "super-secret")
	require.NotContains(t, result.Errors[0].Message, "access_token")
	require.NotContains(t, result.Errors[0].Message, "mailbox_url")
}

func TestCardMailboxImportJSONLRejectsUnsafeURLs(t *testing.T) {
	tests := []string{
		`{"email":"local@example.com","mailbox_url":"http://localhost/inbox"}`,
		`{"email":"loopback@example.com","mailbox_url":"https://127.0.0.1/inbox"}`,
		`{"email":"private@example.com","mailbox_url":"https://10.0.0.5/inbox"}`,
		`{"email":"metadata@example.com","mailbox_url":"http://169.254.169.254/latest/meta-data"}`,
		`{"email":"file@example.com","mailbox_url":"file:///tmp/inbox"}`,
		`{"email":"ipv6loop@example.com","mailbox_url":"https://[::1]/inbox"}`,
		`{"email":"ipv6link@example.com","mailbox_url":"https://[fe80::1]/inbox"}`,
		`{"email":"ipv6private@example.com","mailbox_url":"https://[fd00::1]/inbox"}`,
	}

	for _, line := range tests {
		repo := &cardMailboxRepoStub{}
		svc := NewCardMailboxService(repo, nil)

		result, err := svc.ImportJSONL(context.Background(), strings.NewReader(line))

		require.NoError(t, err)
		require.Equal(t, 0, result.Imported)
		require.Equal(t, 1, result.Failed)
		require.Empty(t, repo.upserts)
	}
}

func TestCardMailboxImportJSONLNilRepoReturnsControlledError(t *testing.T) {
	svc := NewCardMailboxService(nil, nil)

	_, err := svc.ImportJSONL(context.Background(), strings.NewReader(`{"email":"user@example.com","mailbox_url":"https://mail.example.com/inbox"}`))

	require.ErrorIs(t, err, ErrCardMailboxDependencyMissing)
}

func TestCardMailboxImportJSONLRedactsRepositoryErrors(t *testing.T) {
	repo := &cardMailboxRepoStub{upsertErr: errors.New("duplicate mailbox_url=https://mail.example.com/?refresh_token=super-secret password=hunter2 token=abc")}
	svc := NewCardMailboxService(repo, nil)

	result, err := svc.ImportJSONL(context.Background(), strings.NewReader(`{"email":"leak@example.com","mailbox_url":"https://mail.example.com/?refresh_token=super-secret"}`))

	require.NoError(t, err)
	require.Equal(t, 0, result.Imported)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Errors, 1)
	message := result.Errors[0].Message
	require.NotContains(t, message, "super-secret")
	require.NotContains(t, message, "hunter2")
	require.NotContains(t, message, "abc")
	require.NotContains(t, message, "mailbox_url")
	require.NotContains(t, message, "refresh_token")
	require.NotContains(t, message, "password")
	require.NotContains(t, message, "token")
}

func TestCardMailboxFetchCodeUsesInjectedHTTPClientParsesResponsesAndPersists(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "json numeric", body: `{"messages":[{"body":"Your verification code is 123456."}]}`, want: "123456"},
		{name: "json numeric field", body: `{"subject":"verification","code":234567}`, want: "234567"},
		{name: "json metadata", body: `{"subject":"Security code 345678","from":"noreply@example.com","received_at":"2026-05-22T10:30:00Z","snippet":"Use 345678 to continue","body":"Full body has 345678"}`, want: "345678"},
		{name: "plain numeric", body: "Use code 765432 to continue", want: "765432"},
		{name: "html numeric", body: "<html><body><p>Verification code: <b>456789</b></p></body></html>", want: "456789"},
		{name: "alphanumeric near keyword", body: "Your verification token is AB12CD for sign in", want: "AB12CD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
				"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://mail.example.com/inbox?refresh_token=secret"},
			}}
			var requestedURL string
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requestedURL = req.URL.String()
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
				}, nil
			})}
			svc := NewCardMailboxService(repo, client)
			svc.resolver = cardMailboxResolverStub{}

			result, err := svc.FetchCode(context.Background(), 11)

			require.NoError(t, err)
			require.Equal(t, tt.want, result.Code)
			require.Equal(t, CardMailboxFetchStatusSuccess, result.Status)
			require.NotContains(t, result.Subject, "refresh_token")
			require.NotContains(t, result.Snippet, "refresh_token")
			if tt.name == "json metadata" {
				require.Equal(t, "subject", result.Source)
				require.Equal(t, "Security code 345678", result.Subject)
				require.Equal(t, "noreply@example.com", result.From)
				require.Equal(t, time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC), result.ReceivedAt)
				require.Equal(t, "Use 345678 to continue", result.Snippet)
			}
			require.Equal(t, "https://mail.example.com/inbox?refresh_token=secret", requestedURL)
			require.Len(t, repo.latestUpdates, 1)
			require.Equal(t, tt.want, repo.latestUpdates[0].LastCode)
			require.Equal(t, CardMailboxFetchStatusSuccess, repo.latestUpdates[0].LastStatus)
			require.Empty(t, repo.latestUpdates[0].LastError)
			require.NotNil(t, repo.latestUpdates[0].LastFetchedAt)
		})
	}
}

func TestCardMailboxFetchCodeRejectsDomainResolvingToPrivateIP(t *testing.T) {
	repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
		"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://safe-looking.example/inbox"},
	}}
	called := false
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("code 123456"))}, nil
	})}
	svc := NewCardMailboxService(repo, client)
	svc.resolver = cardMailboxResolverStub{ips: map[string][]net.IPAddr{
		"safe-looking.example": {{IP: net.ParseIP("10.0.0.5")}},
	}}

	_, err := svc.FetchCode(context.Background(), 11)

	require.ErrorIs(t, err, ErrCardMailboxInvalidInput)
	require.False(t, called)
	require.Len(t, repo.latestUpdates, 1)
	require.Equal(t, CardMailboxFetchStatusFailed, repo.latestUpdates[0].LastStatus)
	require.NotNil(t, repo.latestUpdates[0].LastFetchedAt)
}

func TestCardMailboxDefaultDialerDialsResolvedValidatedIP(t *testing.T) {
	resolver := cardMailboxResolverStub{ips: map[string][]net.IPAddr{
		"mail.example.com": {{IP: net.ParseIP("8.8.8.8")}},
	}}
	var dialedAddress string
	client := newCardMailboxHTTPClientWithDialer(resolver, func(ctx context.Context, network, address string) (net.Conn, error) {
		dialedAddress = address
		return nil, errors.New("stop after dial address capture")
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://mail.example.com/inbox", nil)
	require.NoError(t, err)

	_, err = client.Do(req)

	require.Error(t, err)
	require.Equal(t, net.JoinHostPort("8.8.8.8", "443"), dialedAddress)
	require.NotContains(t, dialedAddress, "mail.example.com")
}

func TestCardMailboxDefaultDialerRejectsDNSResolvedIPv6Private(t *testing.T) {
	resolver := cardMailboxResolverStub{ips: map[string][]net.IPAddr{
		"mail.example.com": {{IP: net.ParseIP("fd00::1")}},
	}}
	var dialed bool
	client := newCardMailboxHTTPClientWithDialer(resolver, func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("should not dial")
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://mail.example.com/inbox", nil)
	require.NoError(t, err)

	_, err = client.Do(req)

	require.Error(t, err)
	require.False(t, dialed)
}

func TestCardMailboxDefaultClientRejectsRedirectToPrivateIP(t *testing.T) {
	resolver := cardMailboxResolverStub{ips: map[string][]net.IPAddr{
		"mail.example.com": {{IP: net.ParseIP("8.8.8.8")}},
	}}
	client := newCardMailboxHTTPClient(resolver)
	redirectURL, err := url.Parse("http://127.0.0.1/latest/meta-data")
	require.NoError(t, err)
	req := &http.Request{URL: redirectURL}

	err = client.CheckRedirect(req, nil)

	require.Error(t, err)
}

func TestCardMailboxFetchCodeUnsafeURLPersistenceFailureIsSurfaced(t *testing.T) {
	repo := &cardMailboxRepoStub{
		byEmail: map[string]*CardMailbox{
			"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "http://127.0.0.1/inbox?password=secret"},
		},
		updateErr: errors.New("write failed access_token=super-secret"),
	}
	svc := NewCardMailboxService(repo, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("code 123456"))}, nil
	})})

	_, err := svc.FetchCode(context.Background(), 11)

	require.ErrorIs(t, err, ErrCardMailboxRepositoryFailure)
	require.NotContains(t, err.Error(), "super-secret")
	require.NotContains(t, err.Error(), "access_token")
}

func TestCardMailboxFetchCodeRejectsUnsafeURLBeforeHTTP(t *testing.T) {
	repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
		"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "http://127.0.0.1/inbox"},
	}}
	called := false
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("code 123456"))}, nil
	})}
	svc := NewCardMailboxService(repo, client)
	svc.resolver = cardMailboxResolverStub{}

	_, err := svc.FetchCode(context.Background(), 11)

	require.ErrorIs(t, err, ErrCardMailboxInvalidInput)
	require.False(t, called)
	require.Len(t, repo.latestUpdates, 1)
	require.Equal(t, CardMailboxFetchStatusFailed, repo.latestUpdates[0].LastStatus)
	require.NotNil(t, repo.latestUpdates[0].LastFetchedAt)
}

func TestCardMailboxFetchCodeNilDependenciesReturnControlledErrors(t *testing.T) {
	var nilSvc *CardMailboxService
	_, err := nilSvc.FetchCode(context.Background(), 11)
	require.ErrorIs(t, err, ErrCardMailboxDependencyMissing)

	svc := NewCardMailboxService(nil, nil)
	_, err = svc.FetchCode(context.Background(), 11)
	require.ErrorIs(t, err, ErrCardMailboxDependencyMissing)
}

func TestCardMailboxFetchCodeDefaultClientHasTimeout(t *testing.T) {
	svc := NewCardMailboxService(&cardMailboxRepoStub{}, nil)
	client, ok := svc.httpClient.(*http.Client)
	require.True(t, ok)
	require.Equal(t, cardMailboxFetchTimeout, client.Timeout)
}

func TestCardMailboxFetchCodePrefersKeywordProximateDigitCode(t *testing.T) {
	repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
		"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://mail.example.com/inbox"},
	}}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("Ticket ID 111111 was created. Your verification code is 222222."))}, nil
	})}
	svc := NewCardMailboxService(repo, client)
	svc.resolver = cardMailboxResolverStub{}

	result, err := svc.FetchCode(context.Background(), 11)

	require.NoError(t, err)
	require.Equal(t, "222222", result.Code)
}

func TestCardMailboxFetchCodePrefersNumericCodeBeforeAlphanumeric(t *testing.T) {
	repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
		"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://mail.example.com/inbox"},
	}}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("Verification token is AB12CD and backup code is 654321"))}, nil
	})}
	svc := NewCardMailboxService(repo, client)
	svc.resolver = cardMailboxResolverStub{}

	result, err := svc.FetchCode(context.Background(), 11)

	require.NoError(t, err)
	require.Equal(t, "654321", result.Code)
}

func TestCardMailboxFetchCodeNon2xxAndNoCodePersistFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr *infraerrors.ApplicationError
	}{
		{name: "non 2xx", status: http.StatusBadGateway, body: "bad gateway", wantErr: ErrCardMailboxFetchFailed},
		{name: "no code", status: http.StatusOK, body: "welcome message without otp", wantErr: ErrCardMailboxCodeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
				"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://mail.example.com/inbox"},
			}}
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}
			svc := NewCardMailboxService(repo, client)
			svc.resolver = cardMailboxResolverStub{}

			_, err := svc.FetchCode(context.Background(), 11)

			require.ErrorIs(t, err, tt.wantErr)
			require.Len(t, repo.latestUpdates, 1)
			require.Equal(t, CardMailboxFetchStatusFailed, repo.latestUpdates[0].LastStatus)
			require.NotNil(t, repo.latestUpdates[0].LastFetchedAt)
		})
	}
}

func TestCardMailboxFetchCodeSurfacesSanitizedFailurePersistenceError(t *testing.T) {
	repo := &cardMailboxRepoStub{
		byEmail: map[string]*CardMailbox{
			"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://mail.example.com/inbox?token=secret"},
		},
		updateErr: errors.New("write failed password=hunter2 refresh_token=super-secret"),
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("no verification message"))}, nil
	})}
	svc := NewCardMailboxService(repo, client)
	svc.resolver = cardMailboxResolverStub{}

	_, err := svc.FetchCode(context.Background(), 11)

	require.ErrorIs(t, err, ErrCardMailboxRepositoryFailure)
	require.NotContains(t, err.Error(), "hunter2")
	require.NotContains(t, err.Error(), "super-secret")
	require.NotContains(t, err.Error(), "password")
	require.NotContains(t, err.Error(), "refresh_token")
}

func TestCardMailboxFetchCodeFailureIsRedactedAndPersisted(t *testing.T) {
	repo := &cardMailboxRepoStub{byEmail: map[string]*CardMailbox{
		"user@example.com": {ID: 11, Email: "user@example.com", MailboxURL: "https://mail.example.com/inbox?password=super-secret"},
	}}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial https://mail.example.com/inbox?password=super-secret: boom")
	})}
	svc := NewCardMailboxService(repo, client)
	svc.resolver = cardMailboxResolverStub{}

	_, err := svc.FetchCode(context.Background(), 11)

	require.Error(t, err)
	require.True(t, infraerrors.IsServiceUnavailable(err))
	require.NotContains(t, err.Error(), "super-secret")
	require.NotContains(t, err.Error(), "password")
	require.Len(t, repo.latestUpdates, 1)
	require.Equal(t, CardMailboxFetchStatusFailed, repo.latestUpdates[0].LastStatus)
	require.Empty(t, repo.latestUpdates[0].LastCode)
	require.NotContains(t, repo.latestUpdates[0].LastError, "super-secret")
	require.NotContains(t, repo.latestUpdates[0].LastError, "password")
}
