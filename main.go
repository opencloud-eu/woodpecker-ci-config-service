package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	libenv "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/yaronf/httpsign"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"gopkg.in/yaml.v3"
)

const (
	legacyStarFilePathname = "testenv/conf/drone.legacy.star"
	legacyYamlFilePathname = "testenv/conf/legacy/%s.yaml"
	workflowFilePathname   = "testenv/conf/clean/%s.yaml"
)

var (
	workflows = []string{
		"1_coding-standard-php8.2",
		"2_check-gherkin-standard",
		"3_check-suites-in-expected-failures",
		"4_build-web-cache",
		"5_build-web-pnpm-cache",
		"6_get-go-bin-cache",
		"7_build_ocis_binary_for_testing",
	}

	env struct {
		Host          string `env:"CONFIG_SERVICE_HOST"`
		PublicKeyFile string `env:"CONFIG_SERVICE_PUBLIC_KEY_FILE"`
	}
)

type config struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func init() {
	// load environment variables from .envrc file
	switch err := godotenv.Overload(".envrc", ".env"); {
	// it's fine if the file does not exist, maybe the environment variables are already set... who knows
	case errors.Is(err, os.ErrNotExist):
		break
	case err != nil:
		log.Fatalf("Error loading .env file: %v", err)
	}

	if err := libenv.ParseWithOptions(&env, libenv.Options{
		RequiredIfNoDef: true,
	}); err != nil {
		log.Fatalf("Error processing environment variables: %v", err)
	}

	// build and persist star configurations...
	// used for a step to step migration to the new configuration format
	{
		configs, err := transpileStarConfigs(legacyStarFilePathname)
		if err != nil {
			log.Fatalf("Error building starlark configurations: %v", err)
		}

		for _, config := range configs {
			if err := writeWorkflow(legacyYamlFilePathname, config); err != nil {
				log.Fatalf("Error writing config: %v", err)
			}
		}
	}
}

func transpileStarConfigs(fileName string) ([]config, error) {
	thread := &starlark.Thread{
		Name: "drone",
		Print: func(_ *starlark.Thread, msg string) {
			log.Printf(msg)
		},
	}
	globals, err := starlark.ExecFileOptions(syntax.LegacyFileOptions(), thread, fileName, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error executing file: %v", err)
	}

	v, err := starlark.Call(thread, globals["main"], []starlark.Value{
		starlarkstruct.FromStringDict(
			starlark.String("context"),
			starlark.StringDict{
				"repo": starlarkstruct.FromStringDict(starlark.String("repo"), starlark.StringDict{
					"name": starlark.String("name"),
					"slug": starlark.String("slug"),
				}),
				"build": starlarkstruct.FromStringDict(starlark.String("build"), starlark.StringDict{
					"event":       starlark.String("event"),
					"title":       starlark.String("title"),
					"commit":      starlark.String("commit"),
					"ref":         starlark.String("ref"),
					"target":      starlark.String("target"),
					"source":      starlark.String("source"),
					"source_repo": starlark.String("source_repo"),
				}),
			},
		),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("error building conf: %v", err)
	}

	// shame on me....
	hacky := v.String()
	hacky = strings.ReplaceAll(hacky, "False", "false")
	hacky = strings.ReplaceAll(hacky, "True", "true")
	hacky = strings.ReplaceAll(hacky, "None", "[]")

	var workflows []interface{}
	if err := json.Unmarshal([]byte(hacky), &workflows); err != nil {
		return nil, err
	}

	var configs []config
	for _, workflow := range workflows {
		var wfb strings.Builder
		enc := yaml.NewEncoder(&wfb)
		enc.SetIndent(2)
		if err := enc.Encode(workflow); err != nil {
			return nil, err
		}

		var transport struct {
			Name string
		}
		wf := wfb.String()
		if err := yaml.Unmarshal([]byte(wf), &transport); err != nil {
			log.Fatalf("error unmarshaling YAML: %v", err)
		}

		configs = append(configs, config{Name: transport.Name, Data: wf})
	}

	return configs, nil
}

func writeWorkflow(pn string, c config) error {
	destination := fmt.Sprintf(pn, strings.ReplaceAll(c.Name, "/", "--"))
	if err := os.MkdirAll(filepath.Dir(destination), 0770); err != nil {
		return err
	}

	f, err := os.Create(destination)
	if err != nil {
		return err
	}

	if _, err = f.WriteString(c.Data); err != nil {
		return err
	}

	return f.Close()
}

func readWorkflow(p, name string) (config, error) {
	f, err := os.ReadFile(fmt.Sprintf(p, name))
	if err != nil {
		return config{}, err
	}

	return config{
		Name: name,
		Data: string(f),
	}, nil
}

func readWorkflows(p string, names []string) ([]config, error) {
	var configs []config
	for _, name := range names {
		config, err := readWorkflow(p, name)
		if err != nil {
			return nil, err
		}

		configs = append(configs, config)
	}

	return configs, nil
}

func requestVerifier(pubKeyPath string) (func(http.HandlerFunc) http.HandlerFunc, error) {
	if pubKeyPath == "" {
		return nil, fmt.Errorf("public key path is empty")
	}

	pubKeyRaw, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(pubKeyRaw)
	b, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}

	pubKey, ok := b.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not of type ed25519")
	}

	verifier, err := httpsign.NewEd25519Verifier(pubKey,
		httpsign.NewVerifyConfig(),
		httpsign.Headers("@request-target", "content-digest"),
	)

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := httpsign.VerifyRequest("woodpecker-ci-extensions", *verifier, r); err != nil {
				http.Error(w, "Invalid signature", http.StatusBadRequest)
				return
			}

			next.ServeHTTP(w, r)
		}
	}, nil
}

func main() {
	verifier, err := requestVerifier(env.PublicKeyFile)
	if err != nil {
		log.Fatalf("Error on check key handler: %v", err)
	}

	http.HandleFunc("/ciconfig", verifier(func(w http.ResponseWriter, r *http.Request) {
		configs, err := readWorkflows(workflowFilePathname, workflows)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err = json.NewEncoder(w).Encode(map[string]interface{}{"configs": configs}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	err = http.ListenAndServe(env.Host, nil)
	if err != nil {
		log.Fatalf("Error on listen: %v", err)
	}
}
