package config

import (
	"strings"
	"testing"
)

// setEnvs sets multiple env vars for the test.
func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

// minimalEnv returns the minimum env vars for a valid config.
func minimalEnv() map[string]string {
	return map[string]string{
		"ARGOCD_BASE_URL": "https://argocd.example.com",
		"ARGOCD_TOKEN":    "test-token",
	}
}

// --- Load tests ---

func TestLoad_MinimalConfig(t *testing.T) {
	setEnvs(t, minimalEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ArgoCDBaseURL != "https://argocd.example.com" {
		t.Errorf("expected base URL, got %q", cfg.ArgoCDBaseURL)
	}
	if cfg.Transport != "stdio" {
		t.Errorf("expected default transport=stdio, got %q", cfg.Transport)
	}
	if cfg.AuthMode != "token" {
		t.Errorf("expected default auth_mode=token, got %q", cfg.AuthMode)
	}
	if cfg.SpecURL != "https://argocd.example.com/swagger.json" {
		t.Errorf("expected auto spec URL, got %q", cfg.SpecURL)
	}
}

func TestLoad_MissingBaseURL(t *testing.T) {
	t.Setenv("ARGOCD_TOKEN", "test-token")
	t.Setenv("ARGOCD_BASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ARGOCD_BASE_URL")
	}
	if !strings.Contains(err.Error(), "ARGOCD_BASE_URL") {
		t.Errorf("expected ARGOCD_BASE_URL in error, got: %v", err)
	}
}

func TestLoad_MissingTokenInTokenMode(t *testing.T) {
	t.Setenv("ARGOCD_BASE_URL", "https://argocd.example.com")
	t.Setenv("ARGOCD_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ARGOCD_TOKEN")
	}
	if !strings.Contains(err.Error(), "ARGOCD_TOKEN") {
		t.Errorf("expected ARGOCD_TOKEN in error, got: %v", err)
	}
}

func TestLoad_InvalidAuthMode(t *testing.T) {
	envs := minimalEnv()
	envs["AUTH_MODE"] = "invalid"
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid AUTH_MODE")
	}
	if !strings.Contains(err.Error(), "AUTH_MODE") {
		t.Errorf("expected AUTH_MODE in error, got: %v", err)
	}
}

func TestLoad_InvalidTransport(t *testing.T) {
	envs := minimalEnv()
	envs["MCP_TRANSPORT"] = "grpc"
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
	if !strings.Contains(err.Error(), "MCP_TRANSPORT") {
		t.Errorf("expected MCP_TRANSPORT in error, got: %v", err)
	}
}

func TestLoad_OAuthRequiresHTTP(t *testing.T) {
	envs := minimalEnv()
	envs["AUTH_MODE"] = "oauth"
	envs["MCP_TRANSPORT"] = "stdio"
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for oauth without http")
	}
	if !strings.Contains(err.Error(), "AUTH_MODE=oauth requires MCP_TRANSPORT=http") {
		t.Errorf("expected oauth/http error, got: %v", err)
	}
}

func TestLoad_OAuthNoTokenRequired(t *testing.T) {
	t.Setenv("ARGOCD_BASE_URL", "https://argocd.example.com")
	t.Setenv("AUTH_MODE", "oauth")
	t.Setenv("MCP_TRANSPORT", "http")
	t.Setenv("ARGOCD_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AuthMode != "oauth" {
		t.Errorf("expected oauth mode, got %q", cfg.AuthMode)
	}
}

func TestLoad_SpecURLOverride(t *testing.T) {
	envs := minimalEnv()
	envs["ARGOCD_SPEC_URL"] = "https://custom.example.com/spec.json"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SpecURL != "https://custom.example.com/spec.json" {
		t.Errorf("expected custom spec URL, got %q", cfg.SpecURL)
	}
}

func TestLoad_ServerBaseURLDefault(t *testing.T) {
	envs := minimalEnv()
	envs["MCP_ADDR"] = ":9090"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ServerBaseURL != "http://localhost:9090" {
		t.Errorf("expected default server base URL, got %q", cfg.ServerBaseURL)
	}
}

func TestLoad_MultipleErrors(t *testing.T) {
	t.Setenv("ARGOCD_BASE_URL", "")
	t.Setenv("ARGOCD_TOKEN", "")
	t.Setenv("AUTH_MODE", "token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ARGOCD_BASE_URL") || !strings.Contains(err.Error(), "ARGOCD_TOKEN") {
		t.Errorf("expected both errors, got: %v", err)
	}
}

func TestLoad_InvalidBoolFailsStartup(t *testing.T) {
	envs := minimalEnv()
	envs["DISABLE_WRITE"] = "oui"
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid boolean DISABLE_WRITE=oui")
	}
	if !strings.Contains(err.Error(), "DISABLE_WRITE") {
		t.Errorf("expected DISABLE_WRITE in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "oui") {
		t.Errorf("expected invalid value in error, got: %v", err)
	}
}

func TestLoad_MultipleInvalidBoolsReportsAll(t *testing.T) {
	envs := minimalEnv()
	envs["DISABLE_WRITE"] = "yes"
	envs["AUDIT_LOG"] = "nope"
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DISABLE_WRITE") {
		t.Errorf("expected DISABLE_WRITE in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "AUDIT_LOG") {
		t.Errorf("expected AUDIT_LOG in error, got: %v", err)
	}
}

// --- parseBool tests ---

func TestParseBool_ValidTrue(t *testing.T) {
	for _, v := range []string{"true", "True", "TRUE", "1", "t", "T"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("TEST_BOOL", v)
			val, err := parseBool("TEST_BOOL", false)
			if !val {
				t.Errorf("expected true for %q", v)
			}
			if err != nil {
				t.Errorf("unexpected error for valid value %q: %v", v, err)
			}
		})
	}
}

func TestParseBool_ValidFalse(t *testing.T) {
	for _, v := range []string{"false", "False", "FALSE", "0", "f", "F"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("TEST_BOOL", v)
			val, err := parseBool("TEST_BOOL", true)
			if val {
				t.Errorf("expected false for %q", v)
			}
			if err != nil {
				t.Errorf("unexpected error for valid value %q: %v", v, err)
			}
		})
	}
}

func TestParseBool_Empty_ReturnsDefault(t *testing.T) {
	t.Setenv("TEST_BOOL", "")
	val, err := parseBool("TEST_BOOL", true)
	if !val {
		t.Error("expected default true")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseBool_Invalid_ReturnsError(t *testing.T) {
	invalidValues := []string{"oui", "yes", "no", "nope", "2", "on", "off"}
	for _, v := range invalidValues {
		t.Run(v, func(t *testing.T) {
			t.Setenv("TEST_BOOL", v)
			_, err := parseBool("TEST_BOOL", false)
			if err == nil {
				t.Errorf("expected error for invalid value %q", v)
			}
			if err != nil && !strings.Contains(err.Error(), v) {
				t.Errorf("expected value in error, got: %v", err)
			}
		})
	}
}

// --- parseCSV tests ---

func TestParseCSV_Empty(t *testing.T) {
	t.Setenv("TEST_CSV", "")
	result := parseCSV("TEST_CSV")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseCSV_SingleValue(t *testing.T) {
	t.Setenv("TEST_CSV", "ApplicationService")
	result := parseCSV("TEST_CSV")
	if len(result) != 1 || result[0] != "ApplicationService" {
		t.Errorf("expected [ApplicationService], got %v", result)
	}
}

func TestParseCSV_MultipleValues(t *testing.T) {
	t.Setenv("TEST_CSV", "ApplicationService,ProjectService,VersionService")
	result := parseCSV("TEST_CSV")
	if len(result) != 3 {
		t.Fatalf("expected 3 values, got %d", len(result))
	}
}

func TestParseCSV_TrimsWhitespace(t *testing.T) {
	t.Setenv("TEST_CSV", " ApplicationService , ProjectService ")
	result := parseCSV("TEST_CSV")
	if len(result) != 2 {
		t.Fatalf("expected 2 values, got %d", len(result))
	}
	if result[0] != "ApplicationService" || result[1] != "ProjectService" {
		t.Errorf("expected trimmed values, got %v", result)
	}
}

func TestParseCSV_SkipsEmptyEntries(t *testing.T) {
	t.Setenv("TEST_CSV", "ApplicationService,,ProjectService,")
	result := parseCSV("TEST_CSV")
	if len(result) != 2 {
		t.Fatalf("expected 2 values (empty entries skipped), got %d: %v", len(result), result)
	}
}

func TestParseCSV_AllWhitespace(t *testing.T) {
	t.Setenv("TEST_CSV", " , , ")
	result := parseCSV("TEST_CSV")
	if len(result) != 0 {
		t.Errorf("expected 0 values for all-whitespace, got %v", result)
	}
}
