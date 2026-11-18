//go:build e2e

package fixtures

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
)

const (
	k3sImage      = "rancher/k3s:v1.36.6-k3s1"
	argoCDVersion = "v3.6.2"
	nodePort      = "30080"

	DemoApp = "guestbook"
)

type ArgoCD struct {
	URL   string
	Token string
}

var (
	mu        sync.Mutex
	shared    *ArgoCD
	container *k3s.K3sContainer
)

func RunArgoCD(t testing.TB) *ArgoCD {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()

	if shared != nil {
		return shared
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	argo, err := bootstrap(ctx, t)
	if err != nil {
		if container != nil {
			if out, dumpErr := kubectl(context.Background(), "get", "pods", "-A", "-o", "wide"); dumpErr == nil {
				t.Logf("cluster state:\n%s", out)
			}
		}
		t.Fatalf("start argocd: %v", err)
	}
	shared = argo
	return shared
}

func CleanupArgoCD() {
	if container != nil {
		_ = testcontainers.TerminateContainer(container)
	}
}

func bootstrap(ctx context.Context, t testing.TB) (*ArgoCD, error) {
	t.Logf("starting %s", k3sImage)
	ctr, err := k3s.Run(ctx, k3sImage, testcontainers.WithExposedPorts(nodePort+"/tcp"))
	container = ctr
	if err != nil {
		return nil, err
	}

	t.Logf("installing argocd %s", argoCDVersion)
	manifest := fmt.Sprintf("https://raw.githubusercontent.com/argoproj/argo-cd/%s/manifests/install.yaml", argoCDVersion)
	steps := [][]string{
		{"create", "namespace", "argocd"},
		{"apply", "-n", "argocd", "--server-side", "--force-conflicts", "-f", manifest},
		{"-n", "argocd", "scale", "--replicas=0",
			"deployment/argocd-dex-server",
			"deployment/argocd-notifications-controller",
			"deployment/argocd-applicationset-controller",
			"deployment/argocd-repo-server",
			"statefulset/argocd-application-controller"},
		{"-n", "argocd", "patch", "configmap", "argocd-cmd-params-cm", "--type", "merge",
			"-p", `{"data":{"server.insecure":"true"}}`},
		{"-n", "argocd", "patch", "service", "argocd-server",
			"-p", `{"spec":{"type":"NodePort","ports":[{"name":"http","port":80,"targetPort":8080,"nodePort":` + nodePort + `}]}}`},
		{"-n", "argocd", "rollout", "restart", "deployment/argocd-server"},
		{"-n", "argocd", "rollout", "status", "deployment/argocd-server", "--timeout=5m"},
	}
	for _, args := range steps {
		start := time.Now()
		if _, err := kubectl(ctx, args...); err != nil {
			return nil, err
		}
		t.Logf("kubectl %s (%s)", strings.Join(args[:min(len(args), 4)], " "), time.Since(start).Round(time.Second))
	}

	if err := seedApplication(ctx); err != nil {
		return nil, err
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("container host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, nodePort+"/tcp")
	if err != nil {
		return nil, fmt.Errorf("mapped port: %w", err)
	}
	url := fmt.Sprintf("http://%s:%s", host, port.Port())

	password, err := adminPassword(ctx)
	if err != nil {
		return nil, err
	}
	token, err := sessionToken(ctx, url, password)
	if err != nil {
		return nil, err
	}

	t.Logf("argocd ready at %s", url)
	return &ArgoCD{URL: url, Token: token}, nil
}

func seedApplication(ctx context.Context) error {
	app := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ` + DemoApp + `
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: guestbook
  destination:
    server: https://kubernetes.default.svc
    namespace: default
`
	if err := container.CopyToContainer(ctx, []byte(app), "/tmp/app.yaml", 0o644); err != nil {
		return fmt.Errorf("copy application manifest: %w", err)
	}
	_, err := kubectl(ctx, "apply", "-f", "/tmp/app.yaml")
	return err
}

func adminPassword(ctx context.Context) (string, error) {
	var lastErr error
	for ctx.Err() == nil {
		out, err := kubectl(ctx, "-n", "argocd", "get", "secret", "argocd-initial-admin-secret",
			"-o", "jsonpath={.data.password}")
		if err == nil && out != "" {
			decoded, err := base64.StdEncoding.DecodeString(out)
			if err != nil {
				return "", fmt.Errorf("decode admin password: %w", err)
			}
			return string(decoded), nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("admin password not available: %w", lastErr)
}

func sessionToken(ctx context.Context, url, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": password})
	var lastErr error
	for ctx.Err() == nil {
		token, err := login(ctx, url, body)
		if err == nil {
			return token, nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("login to argocd: %w", lastErr)
}

func login(ctx context.Context, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/v1/session", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode session: %w", err)
	}
	return out.Token, nil
}

func kubectl(ctx context.Context, args ...string) (string, error) {
	code, reader, err := container.Exec(ctx, append([]string{"kubectl"}, args...), tcexec.Multiplexed())
	if err != nil {
		return "", fmt.Errorf("kubectl %s: %w", strings.Join(args, " "), err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("kubectl %s: read output: %w", strings.Join(args, " "), err)
	}
	if code != 0 {
		return "", fmt.Errorf("kubectl %s: exit %d: %s", strings.Join(args, " "), code, out)
	}
	return strings.TrimSpace(string(out)), nil
}
