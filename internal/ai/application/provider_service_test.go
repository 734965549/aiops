package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeProviderStore struct {
	records map[string]domain.Provider
}

func newFakeProviderStore() *fakeProviderStore {
	return &fakeProviderStore{records: map[string]domain.Provider{}}
}

func (f *fakeProviderStore) List(_ context.Context) ([]domain.Provider, error) {
	ids := make([]string, 0, len(f.records))
	for id := range f.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]domain.Provider, 0, len(ids))
	for _, id := range ids {
		out = append(out, f.records[id])
	}
	return out, nil
}

func (f *fakeProviderStore) Get(_ context.Context, providerID string) (*domain.Provider, error) {
	record, ok := f.records[providerID]
	if !ok {
		return nil, domain.ErrProviderNotFound
	}
	out := record
	return &out, nil
}

func (f *fakeProviderStore) Upsert(_ context.Context, provider domain.Provider) error {
	f.records[provider.ProviderID] = provider
	return nil
}

func (f *fakeProviderStore) Delete(_ context.Context, providerID string) error {
	delete(f.records, providerID)
	return nil
}

type fakeProviderRuntime struct {
	providers   map[string]toolgateway.ProviderConfig
	validateErr error
}

func newFakeProviderRuntime() *fakeProviderRuntime {
	return &fakeProviderRuntime{providers: map[string]toolgateway.ProviderConfig{}}
}

func (f *fakeProviderRuntime) ValidateProvider(cfg toolgateway.ProviderConfig) error {
	return f.validateErr
}

func (f *fakeProviderRuntime) UpsertProvider(cfg toolgateway.ProviderConfig) error {
	if f.validateErr != nil {
		return f.validateErr
	}
	f.providers[cfg.ID] = cfg
	return nil
}

func (f *fakeProviderRuntime) DeleteProvider(id string) { delete(f.providers, id) }

type fakeSecretCipher struct{}

func (fakeSecretCipher) Encrypt(plaintext string) ([]byte, error) {
	return []byte("enc:" + plaintext), nil
}

func (fakeSecretCipher) Decrypt(ciphertext []byte) (string, error) {
	raw := string(ciphertext)
	if !strings.HasPrefix(raw, "enc:") {
		return "", errors.New("invalid ciphertext")
	}
	return strings.TrimPrefix(raw, "enc:"), nil
}

func TestProviderServiceBootstrapSeedsWhenDatabaseEmpty(t *testing.T) {
	store := newFakeProviderStore()
	runtime := newFakeProviderRuntime()
	svc := NewProviderService(store, runtime, fakeSecretCipher{})

	err := svc.Bootstrap(context.Background(), []ProviderSeed{{
		ID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com",
		Model: "ep-1", APIKey: "seed-key", Enabled: true,
	}})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	record, ok := store.records["ark"]
	if !ok {
		t.Fatal("expected seed persisted")
	}
	if string(record.APIKeyCiphertext) != "enc:seed-key" || !record.HasAPIKey {
		t.Fatalf("expected encrypted seed key, got %q", string(record.APIKeyCiphertext))
	}
	cfg, ok := runtime.providers["ark"]
	if !ok || cfg.APIKey != "seed-key" || cfg.Model != "ep-1" {
		t.Fatalf("unexpected runtime provider: %+v", cfg)
	}
}

func TestProviderServiceBootstrapPrefersDatabaseOverSeeds(t *testing.T) {
	store := newFakeProviderStore()
	store.records["db"] = domain.Provider{
		ProviderID: "db", Name: "DB", Type: "b", BaseURL: "https://ark.example.com",
		Model: "ep-db", APIKeyCiphertext: []byte("enc:db-key"), HasAPIKey: true, Enabled: true,
	}
	runtime := newFakeProviderRuntime()
	svc := NewProviderService(store, runtime, fakeSecretCipher{})

	if err := svc.Bootstrap(context.Background(), []ProviderSeed{{
		ID: "seed", Name: "Seed", Type: "b", BaseURL: "https://ark.example.com", Model: "ep-seed",
		APIKey: "seed-key", Enabled: true,
	}}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, ok := store.records["seed"]; ok {
		t.Fatal("config seed must not be written when database is not empty")
	}
	if cfg, ok := runtime.providers["db"]; !ok || cfg.APIKey != "db-key" {
		t.Fatalf("expected database provider loaded, got %+v", runtime.providers)
	}
}

func TestProviderServiceBootstrapSkipsUndecryptableProvider(t *testing.T) {
	store := newFakeProviderStore()
	store.records["broken"] = domain.Provider{
		ProviderID: "broken", Type: "b", BaseURL: "https://ark.example.com",
		Model: "ep-1", APIKeyCiphertext: []byte("plain"), HasAPIKey: true, Enabled: true,
	}
	runtime := newFakeProviderRuntime()
	svc := NewProviderService(store, runtime, fakeSecretCipher{})

	if err := svc.Bootstrap(context.Background(), nil); err != nil {
		t.Fatalf("bootstrap must not fail on one bad provider: %v", err)
	}
	if len(runtime.providers) != 0 {
		t.Fatalf("expected broken provider skipped, got %+v", runtime.providers)
	}
}

func TestProviderServiceUpsertKeepsExistingAPIKey(t *testing.T) {
	store := newFakeProviderStore()
	store.records["ark"] = domain.Provider{
		ProviderID: "ark", Name: "Old", Type: "b", BaseURL: "https://ark.example.com",
		Model: "ep-1", APIKeyCiphertext: []byte("enc:stored-key"), HasAPIKey: true,
	}
	runtime := newFakeProviderRuntime()
	svc := NewProviderService(store, runtime, fakeSecretCipher{})

	if err := svc.UpsertProvider(context.Background(), UpsertProviderInput{
		ID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com", Model: "ep-2", Enabled: true,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := string(store.records["ark"].APIKeyCiphertext); got != "enc:stored-key" {
		t.Fatalf("expected original ciphertext preserved, got %q", got)
	}
	if cfg, ok := runtime.providers["ark"]; !ok || cfg.APIKey != "stored-key" || cfg.Model != "ep-2" {
		t.Fatalf("unexpected runtime provider: %+v", cfg)
	}
}

func TestProviderServiceUpsertRequiresKeyForNewProvider(t *testing.T) {
	svc := NewProviderService(newFakeProviderStore(), newFakeProviderRuntime(), fakeSecretCipher{})
	err := svc.UpsertProvider(context.Background(), UpsertProviderInput{
		ID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com", Model: "ep-1",
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestProviderServiceListMasksAPIKey(t *testing.T) {
	store := newFakeProviderStore()
	store.records["ark"] = domain.Provider{
		ProviderID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com",
		Model: "ep-1", APIKeyCiphertext: []byte("enc:secret"), HasAPIKey: true, Enabled: true,
	}
	svc := NewProviderService(store, newFakeProviderRuntime(), fakeSecretCipher{})
	list, err := svc.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || !list[0].HasAPIKey || list[0].Model != "ep-1" {
		t.Fatalf("unexpected public provider: %+v", list)
	}
}

func TestProviderServiceDeleteRemovesRuntime(t *testing.T) {
	store := newFakeProviderStore()
	store.records["ark"] = domain.Provider{
		ProviderID: "ark", Type: "b", BaseURL: "https://ark.example.com", Model: "ep-1",
		APIKeyCiphertext: []byte("enc:key"), HasAPIKey: true,
	}
	runtime := newFakeProviderRuntime()
	svc := NewProviderService(store, runtime, fakeSecretCipher{})
	if err := svc.Bootstrap(context.Background(), nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if err := svc.DeleteProvider(context.Background(), "ark"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := store.records["ark"]; ok {
		t.Fatal("expected database record deleted")
	}
	if _, ok := runtime.providers["ark"]; ok {
		t.Fatal("expected runtime provider removed")
	}
}

func TestProviderServiceUpsertPropagatesRuntimeValidation(t *testing.T) {
	runtime := newFakeProviderRuntime()
	runtime.validateErr = apperr.New(apperr.CodeInvalidArgument, "model is required")
	svc := NewProviderService(newFakeProviderStore(), runtime, fakeSecretCipher{})
	err := svc.UpsertProvider(context.Background(), UpsertProviderInput{
		ID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com", APIKey: "k",
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected validation error, got %v", err)
	}
}
