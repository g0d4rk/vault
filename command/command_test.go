package command

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/audit"
	"github.com/hashicorp/vault/builtin/logical/pki"
	"github.com/hashicorp/vault/builtin/logical/ssh"
	"github.com/hashicorp/vault/builtin/logical/transit"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/physical/inmem"
	"github.com/hashicorp/vault/vault"
	"github.com/mitchellh/cli"

	auditFile "github.com/hashicorp/vault/builtin/audit/file"
	credUserpass "github.com/hashicorp/vault/builtin/credential/userpass"
	vaulthttp "github.com/hashicorp/vault/http"
	logxi "github.com/mgutz/logxi/v1"
)

var (
	defaultVaultLogger = logxi.NullLog

	defaultVaultCredentialBackends = map[string]logical.Factory{
		"userpass": credUserpass.Factory,
	}

	defaultVaultAuditBackends = map[string]audit.Factory{
		"file": auditFile.Factory,
	}

	defaultVaultLogicalBackends = map[string]logical.Factory{
		"generic-leased": vault.LeasedPassthroughBackendFactory,
		"pki":            pki.Factory,
		"ssh":            ssh.Factory,
		"transit":        transit.Factory,
	}
)

// assertNoTabs asserts the CLI help has no tab characters.
func assertNoTabs(tb testing.TB, c cli.Command) {
	tb.Helper()

	if strings.ContainsRune(c.Help(), '\t') {
		tb.Errorf("%#v help output contains tabs", c)
	}
}

// testVaultServer creates a test vault cluster and returns a configured API
// client and closer function.
func testVaultServer(tb testing.TB) (*api.Client, func()) {
	tb.Helper()

	client, _, closer := testVaultServerUnseal(tb)
	return client, closer
}

// testVaultServerUnseal creates a test vault cluster and returns a configured
// API client, list of unseal keys (as strings), and a closer function.
func testVaultServerUnseal(tb testing.TB) (*api.Client, []string, func()) {
	tb.Helper()

	return testVaultServerCoreConfig(tb, &vault.CoreConfig{
		DisableMlock:       true,
		DisableCache:       true,
		Logger:             defaultVaultLogger,
		CredentialBackends: defaultVaultCredentialBackends,
		AuditBackends:      defaultVaultAuditBackends,
		LogicalBackends:    defaultVaultLogicalBackends,
	})
}

// testVaultServerCoreConfig creates a new vault cluster with the given core
// configuration. This is a lower-level test helper.
func testVaultServerCoreConfig(tb testing.TB, coreConfig *vault.CoreConfig) (*api.Client, []string, func()) {
	tb.Helper()

	cluster := vault.NewTestCluster(tb, coreConfig, &vault.TestClusterOptions{
		HandlerFunc: vaulthttp.Handler,
		NumCores:    1, // Default is 3, but we don't need that many
	})
	cluster.Start()

	// Make it easy to get access to the active
	core := cluster.Cores[0].Core
	vault.TestWaitActive(tb, core)

	// Get the client already setup for us!
	client := cluster.Cores[0].Client
	client.SetToken(cluster.RootToken)

	// Convert the unseal keys to base64 encoded, since these are how the user
	// will get them.
	unsealKeys := make([]string, len(cluster.BarrierKeys))
	for i := range unsealKeys {
		unsealKeys[i] = base64.StdEncoding.EncodeToString(cluster.BarrierKeys[i])
	}

	return client, unsealKeys, func() { defer cluster.Cleanup() }
}

// testVaultServerUninit creates an uninitialized server.
func testVaultServerUninit(tb testing.TB) (*api.Client, func()) {
	tb.Helper()

	inm, err := inmem.NewInmem(nil, defaultVaultLogger)
	if err != nil {
		tb.Fatal(err)
	}

	core, err := vault.NewCore(&vault.CoreConfig{
		DisableMlock:       true,
		DisableCache:       true,
		Logger:             defaultVaultLogger,
		Physical:           inm,
		CredentialBackends: defaultVaultCredentialBackends,
		AuditBackends:      defaultVaultAuditBackends,
		LogicalBackends:    defaultVaultLogicalBackends,
	})
	if err != nil {
		tb.Fatal(err)
	}

	ln, addr := vaulthttp.TestServer(tb, core)

	client, err := api.NewClient(&api.Config{
		Address: addr,
	})
	if err != nil {
		tb.Fatal(err)
	}

	return client, func() { ln.Close() }
}

// testVaultServerBad creates an http server that returns a 500 on each request
// to simulate failures.
func testVaultServerBad(tb testing.TB) (*api.Client, func()) {
	tb.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}

	server := &http.Server{
		Addr: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
		}),
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      1 * time.Second,
		IdleTimeout:       1 * time.Second,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			tb.Fatal(err)
		}
	}()

	client, err := api.NewClient(&api.Config{
		Address: "http://" + listener.Addr().String(),
	})
	if err != nil {
		tb.Fatal(err)
	}

	return client, func() {
		ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()

		server.Shutdown(ctx)
	}
}

// testTokenAndAccessor creates a new authentication token capable of being renewed with
// the default policy attached. It returns the token and it's accessor.
func testTokenAndAccessor(tb testing.TB, client *api.Client) (string, string) {
	tb.Helper()

	secret, err := client.Auth().Token().Create(&api.TokenCreateRequest{
		Policies: []string{"default"},
		TTL:      "30m",
	})
	if err != nil {
		tb.Fatal(err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		tb.Fatalf("missing auth data: %#v", secret)
	}
	return secret.Auth.ClientToken, secret.Auth.Accessor
}

func testClient(tb testing.TB, addr string, token string) *api.Client {
	tb.Helper()
	config := api.DefaultConfig()
	config.Address = addr
	client, err := api.NewClient(config)
	if err != nil {
		tb.Fatal(err)
	}
	client.SetToken(token)

	return client
}
