package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/yaronf/httpsign"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"gopkg.in/yaml.v3"
)

const (
	ConfigLegacyStarFilePathname  = "testenv/conf/drone.legacy.star"
	ConfigLegacyYamlFilePathname  = "testenv/conf/legacy/%s.yaml"
	ConfigCleanedYamlFilePathname = "testenv/conf/clean/%s.yaml"
)

var (
	WoodpeckerWorkflows = []string{
		"1_coding-standard-php8.2",
		"2_check-gherkin-standard",
		"3_check-suites-in-expected-failures",
		"4_build-web-cache",
		"5_build-web-pnpm-cache",
		"6_get-go-bin-cache",
		"7_build_ocis_binary_for_testing",
	}
)

type config struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func init() {
	// load environment variables from .envrc file
	err := godotenv.Load(".envrc")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// build and persist star configurations...
	// used for a step to step migration to the new configuration format
	{
		configs, err := transpileStarConfigs(ConfigLegacyStarFilePathname)
		if err != nil {
			log.Fatalf("error building starlark configurations: %v", err)
		}

		for _, config := range configs {
			if err := writeConfig(ConfigLegacyYamlFilePathname, config); err != nil {
				log.Fatalf("error writing config: %v", err)
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

func writeConfig(pn string, c config) error {
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

func loadConfig(pname, fname string) (config, error) {
	f, err := os.ReadFile(fmt.Sprintf(pname, fname))
	if err != nil {
		return config{}, err
	}

	return config{
		Name: fname,
		Data: string(f),
	}, nil
}

func loadConfigs(pname string, fnames []string) ([]config, error) {
	var configs []config
	for _, fname := range fnames {
		config, err := loadConfig(pname, fname)
		if err != nil {
			return nil, err
		}

		configs = append(configs, config)
	}

	return configs, nil
}

func checkKeyHandler(pubKeyPath string) (func(http.HandlerFunc) http.HandlerFunc, error) {
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
	keyCheckHandler, err := checkKeyHandler(os.Getenv("CONFIG_SERVICE_PUBLIC_KEY_FILE"))
	if err != nil {
		log.Fatalf("Error on check key handler: %v", err)
	}

	http.HandleFunc("/ciconfig", keyCheckHandler(func(w http.ResponseWriter, r *http.Request) {
		configs, err := loadConfigs(ConfigCleanedYamlFilePathname, WoodpeckerWorkflows)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(map[string]interface{}{"configs": configs})
	}))

	err = http.ListenAndServe(os.Getenv("CONFIG_SERVICE_HOST"), nil)
	if err != nil {
		log.Fatalf("Error on listen: %v", err)
	}
}
