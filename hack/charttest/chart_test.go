//go:build chart

package charttest

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"testing"

	"go.yaml.in/yaml/v3"
)

type container struct {
	Env []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"env"`
	VolumeMounts []struct {
		Name      string `yaml:"name"`
		MountPath string `yaml:"mountPath"`
	} `yaml:"volumeMounts"`
}

type deployment struct {
	Kind string `yaml:"kind"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []container `yaml:"containers"`
				Volumes    []struct {
					Name string `yaml:"name"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func render(t *testing.T, sets ...string) deployment {
	t.Helper()
	args := []string{"template", "argocd-mcp", "../../charts/argocd-mcp", "--namespace", "argocd-mcp",
		"--set", "argocd.baseURL=https://argocd.example.com", "--set", "argocd.token=test"}
	for _, s := range sets {
		args = append(args, "--set-json", s)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}

	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var d deployment
		if err := dec.Decode(&d); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode manifests: %v", err)
		}
		if d.Kind == "Deployment" {
			return d
		}
	}
	t.Fatal("no Deployment rendered")
	return deployment{}
}

func envValue(c container, name string) (string, bool) {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func TestDefaults_NoExtras(t *testing.T) {
	d := render(t)
	c := d.Spec.Template.Spec.Containers[0]
	if _, ok := envValue(c, "ARGOCD_CA_BUNDLE"); ok {
		t.Error("ARGOCD_CA_BUNDLE rendered without argocd.caBundlePath")
	}
	if len(c.VolumeMounts) != 0 || len(d.Spec.Template.Spec.Volumes) != 0 {
		t.Errorf("unexpected volumes %v / mounts %v", d.Spec.Template.Spec.Volumes, c.VolumeMounts)
	}
}

func TestCABundlePath(t *testing.T) {
	c := render(t, `argocd.caBundlePath="/etc/ssl/argocd/ca.pem"`).Spec.Template.Spec.Containers[0]
	if v, _ := envValue(c, "ARGOCD_CA_BUNDLE"); v != "/etc/ssl/argocd/ca.pem" {
		t.Errorf("ARGOCD_CA_BUNDLE = %q, want /etc/ssl/argocd/ca.pem", v)
	}
}

func TestExtraEnvVolumesAndMounts(t *testing.T) {
	d := render(t,
		`extraEnv=[{"name":"HTTPS_PROXY","value":"http://proxy:3128"}]`,
		`extraVolumes=[{"name":"argocd-ca","secret":{"secretName":"argocd-ca-bundle"}}]`,
		`extraVolumeMounts=[{"name":"argocd-ca","mountPath":"/etc/ssl/argocd","readOnly":true}]`,
	)
	c := d.Spec.Template.Spec.Containers[0]

	if v, _ := envValue(c, "HTTPS_PROXY"); v != "http://proxy:3128" {
		t.Errorf("extraEnv not rendered, HTTPS_PROXY = %q", v)
	}
	if v, _ := envValue(c, "ARGOCD_BASE_URL"); v != "https://argocd.example.com" {
		t.Errorf("extraEnv replaced the built-in env, ARGOCD_BASE_URL = %q", v)
	}
	if len(d.Spec.Template.Spec.Volumes) != 1 || d.Spec.Template.Spec.Volumes[0].Name != "argocd-ca" {
		t.Errorf("extraVolumes not rendered: %v", d.Spec.Template.Spec.Volumes)
	}
	if len(c.VolumeMounts) != 1 || c.VolumeMounts[0].MountPath != "/etc/ssl/argocd" {
		t.Errorf("extraVolumeMounts not rendered: %v", c.VolumeMounts)
	}
}
