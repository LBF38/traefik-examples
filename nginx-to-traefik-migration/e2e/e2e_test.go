package e2e

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Configuration variables - can be overridden via command-line flags
var (
	// kubectxName is the name of the kubernetes context to use
	kubectxName string

	// ingressControllerPort is the port where the ingress controller is accessible
	ingressControllerPort string

	// ingressControllerHost is the base host for the ingress controller
	ingressControllerHost string

	// testNamespace is the namespace where test resources will be deployed
	testNamespace string

	ingressControllerNamespace string
)

var fixturesDir string

func init() {
	_, filename, _, _ := runtime.Caller(0)
	fixturesDir = filepath.Join(filepath.Dir(filename), "fixtures")

	// Define command-line flags
	flag.StringVar(&kubectxName, "kubectx", "k3d-traefik-cluster", "Kubernetes context to use")
	// flag.StringVar(&kubectxName, "kubectx", "k3d-k3s-default", "Kubernetes context to use")

	flag.StringVar(&ingressControllerPort, "port", "9080", "Ingress controller port")
	// flag.StringVar(&ingressControllerPort, "port", "80", "Ingress controller port")

	flag.StringVar(&ingressControllerHost, "host", "localhost", "Ingress controller host")
	flag.StringVar(&testNamespace, "namespace", "default", "Namespace for test resources")
	flag.StringVar(&ingressControllerNamespace, "ingress-controller-namespace", "traefik", "Namespace for test resources")
}

func TestMain(m *testing.M) {
	// Parse flags (needed when running with go test)
	flag.Parse()

	fmt.Printf("Using kubectx: %s, port: %s, host: %s, namespace: %s\n",
		kubectxName, ingressControllerPort, ingressControllerHost, testNamespace)

	// Switch to the configured kubectx
	if err := runCommand("kubectx", kubectxName); err != nil {
		panic(fmt.Sprintf("failed to switch kubectx: %v", err))
	}

	// Deploy shared service and deployment once
	if err := deploySharedResources(); err != nil {
		panic(fmt.Sprintf("failed to deploy shared resources: %v", err))
	}

	// Run tests
	code := m.Run()

	// Cleanup shared resources
	cleanupSharedResources()

	os.Exit(code)
}

func deploySharedResources() error {
	if err := applyFixture("deployment.yaml"); err != nil {
		return fmt.Errorf("failed to deploy deployment: %w", err)
	}
	if err := applyFixture("service.yaml"); err != nil {
		return fmt.Errorf("failed to deploy service: %w", err)
	}
	return nil
}

func cleanupSharedResources() {
	_ = runCommand("kubectl", "delete", "deployment", "snippet-test-backend", "-n", testNamespace, "--ignore-not-found")
	_ = runCommand("kubectl", "delete", "service", "snippet-test-backend", "-n", testNamespace, "--ignore-not-found")
}

func Test_Snippet_ResponseHeaders(t *testing.T) {
	testCases := []struct {
		desc                 string
		serverSnippet        string
		configurationSnippet string
		expectedHeaders      map[string]string
	}{
		{
			desc:            "add_header server snippet adds simple header",
			serverSnippet:   `add_header X-Custom "custom-value";`,
			expectedHeaders: map[string]string{"X-Custom": "custom-value"},
		},
		{
			desc:            "add_header server snippet adds header without quotes",
			serverSnippet:   `add_header X-Simple simple;`,
			expectedHeaders: map[string]string{"X-Simple": "simple"},
		},
		{
			desc:                 "add_header configuration snippet adds simple header",
			configurationSnippet: `add_header X-Custom "custom-value";`,
			expectedHeaders:      map[string]string{"X-Custom": "custom-value"},
		},
		{
			desc:                 "add_header configuration snippet adds header without quotes",
			configurationSnippet: `add_header X-Simple simple;`,
			expectedHeaders:      map[string]string{"X-Simple": "simple"},
		},
		{
			desc:                 "add_header configuration snippet overrides server snippet",
			serverSnippet:        `add_header X-Server server-value;`,
			configurationSnippet: `add_header X-Config config-value;`,
			expectedHeaders: map[string]string{
				"X-Server": "",
				"X-Config": "config-value",
			},
		},
		{
			desc:            "more_set_headers server snippet sets header",
			serverSnippet:   `more_set_headers "X-Custom:custom-value";`,
			expectedHeaders: map[string]string{"X-Custom": "custom-value"},
		},
		{
			desc:                 "more_set_headers configuration snippet sets header",
			configurationSnippet: `more_set_headers "X-Custom:custom-value";`,
			expectedHeaders:      map[string]string{"X-Custom": "custom-value"},
		},
		{
			desc:                 "more_set_headers both snippets set headers",
			serverSnippet:        `more_set_headers "X-Server:server-value";`,
			configurationSnippet: `more_set_headers "X-Config:config-value";`,
			expectedHeaders: map[string]string{
				"X-Server": "server-value",
				"X-Config": "config-value",
			},
		},
		{
			desc:                 "more_set_headers both snippets override same header",
			serverSnippet:        `more_set_headers "X-Header:server-value";`,
			configurationSnippet: `more_set_headers "X-Header:config-value";`,
			expectedHeaders: map[string]string{
				"X-Header": "config-value",
			},
		},
		{
			desc:                 "more_set_headers both snippets override same header",
			configurationSnippet: `more_set_headers "X-Header: config-value";`,
			expectedHeaders: map[string]string{
				"X-Header": "config-value",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			ingressName := "snippet-test-" + sanitizeName(test.desc)
			host := ingressName + "." + ingressControllerHost

			// Deploy ingress with snippet annotations
			err := deployIngress(ingressName, host, test.serverSnippet, test.configurationSnippet)
			require.NoError(t, err)

			// Cleanup ingress after test
			defer func() {
				_ = deleteIngress(ingressName)
			}()

			// Wait for ingress to be ready and make request
			resp := makeRequestWithRetry(t, host, 20, 500*time.Millisecond)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			for header, expectedValue := range test.expectedHeaders {
				fmt.Println(resp.Header.Get(header))
				assert.Equal(t, expectedValue, strings.ReplaceAll(resp.Header.Get(header), host, "{{HOST}}"), "header %s mismatch", header)
			}
		})
	}
}

type ingressTemplateData struct {
	Name                 string
	Host                 string
	ServerSnippet        string
	ConfigurationSnippet string
}

func deployIngress(name, host, serverSnippet, configurationSnippet string) error {
	tmplPath := filepath.Join(fixturesDir, "ingress.yaml.tmpl")
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("failed to read ingress template: %w", err)
	}

	// Create template with custom indent function
	tmpl, err := template.New("ingress").Funcs(template.FuncMap{
		"indent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = pad + line
				}
			}
			return strings.Join(lines, "\n")
		},
	}).Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse ingress template: %w", err)
	}

	// Normalize snippets: replace tabs with spaces and trim
	serverSnippet = strings.ReplaceAll(strings.TrimSpace(serverSnippet), "\t", "  ")
	configurationSnippet = strings.ReplaceAll(strings.TrimSpace(configurationSnippet), "\t", "  ")

	data := ingressTemplateData{
		Name:                 name,
		Host:                 host,
		ServerSnippet:        serverSnippet,
		ConfigurationSnippet: configurationSnippet,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute ingress template: %w", err)
	}

	fmt.Println(buf.String())

	return applyManifest(buf.String())
}

func deleteIngress(name string) error {
	return runCommand("kubectl", "delete", "ingress", name, "-n", testNamespace, "--ignore-not-found")
}

func applyFixture(filename string) error {
	fixturePath := filepath.Join(fixturesDir, filename)
	return runCommand("kubectl", "apply", "-f", fixturePath, "-n", testNamespace)
}

func applyManifest(manifest string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-", "-n", testNamespace)
	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %v, output: %s", err, string(output))
	}
	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %s failed: %v, output: %s", name, err, string(output))
	}
	return nil
}

func makeRequestWithRetry(t *testing.T, host string, maxRetries int, delay time.Duration) *http.Response {
	t.Helper()

	url := fmt.Sprintf("http://%s:%s/", ingressControllerHost, ingressControllerPort)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return resp
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		time.Sleep(delay)
	}

	t.Logf("request failed after %d retries: %v", maxRetries, lastErr)
	return nil
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any characters that are not alphanumeric or hyphens
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	// Truncate to max 63 characters (Kubernetes name limit)
	s := result.String()
	if len(s) > 63 {
		s = s[:63]
	}

	// Remove trailing hyphens
	s = strings.TrimRight(s, "-")

	return s
}

func Test_AllDirectives(t *testing.T) {
	testCases := []struct {
		desc                    string
		serverSnippet           string
		configurationSnippet    string
		method                  string
		path                    string
		requestHeaders          map[string]string
		expectedResponseHeaders map[string]string
		expectedRequestHeaders  map[string]string
		expectedStatusCode      int
		expectedBody            string
		// dynamicHost is set to true when the expected value contains the test host
		dynamicHost bool
	}{
		{
			desc: "add_header with variable interpolation",
			configurationSnippet: `
add_header X-Method $request_method;
add_header X-Uri $request_uri;
`,
			expectedResponseHeaders: map[string]string{
				"X-Method": "GET",
				"X-Uri":    "/test",
			},
		},
		{
			desc: "more_set_headers directive",
			configurationSnippet: `
more_set_headers "X-Custom-Header:custom-value";
more_set_headers "X-Another:another-value";
`,
			expectedResponseHeaders: map[string]string{
				"X-Custom-Header": "custom-value",
				"X-Another":       "another-value",
			},
		},
		{
			desc: "proxy_set_header directive",
			configurationSnippet: `
proxy_set_header X-Custom-Method $request_method;
proxy_set_header X-Custom-Uri $request_uri;
`,
			expectedRequestHeaders: map[string]string{
				"X-Custom-Method": "GET",
				"X-Custom-Uri":    "/test",
			},
		},
		{
			desc: "set directive creates variable",
			configurationSnippet: `
set $my_var "hello";
add_header X-My-Var $my_var;
`,
			expectedResponseHeaders: map[string]string{
				"X-My-Var": "hello",
			},
		},
		{
			desc: "set directive with variable interpolation",
			configurationSnippet: `
set $combined "$request_method-$request_uri";
add_header X-Combined $combined;
`,
			expectedResponseHeaders: map[string]string{
				"X-Combined": "GET-/test",
			},
		},
		{
			desc: "if directive with matching condition",
			configurationSnippet: `
if ($request_method = GET) {
	add_header X-Is-Get "true";
}
`,
			method: http.MethodGet,
			expectedResponseHeaders: map[string]string{
				"X-Is-Get": "true",
			},
		},
		{
			desc: "if directive with non-matching condition",
			configurationSnippet: `
if ($request_method = POST) {
	add_header X-Is-Post "true";
}
add_header X-Always "present";
`,
			method: http.MethodGet,
			expectedResponseHeaders: map[string]string{
				"X-Is-Post": "",
				"X-Always":  "present",
			},
		},
		{
			desc: "if directive with header check",
			configurationSnippet: `
if ($http_x_custom = "expected") {
	add_header X-Matched "yes";
}
`,
			requestHeaders: map[string]string{
				"X-Custom": "expected",
			},
			expectedResponseHeaders: map[string]string{
				"X-Matched": "yes",
			},
		},
		{
			desc: "if directive with regex match",
			configurationSnippet: `
if ($request_uri ~ "^/api") {
	add_header X-Is-Api "true";
}
`,
			path: "/api/users",
			expectedResponseHeaders: map[string]string{
				"X-Is-Api": "true",
			},
		},
		{
			desc: "if directive with case-insensitive regex match - matching",
			configurationSnippet: `
if ($http_x_custom ~* "^test") {
	add_header X-Matched "yes";
}
`,
			requestHeaders: map[string]string{
				"X-Custom": "TEST-value",
			},
			expectedResponseHeaders: map[string]string{
				"X-Matched": "yes",
			},
		},
		{
			desc: "if directive with case-insensitive regex match - not matching",
			configurationSnippet: `
if ($http_x_custom ~* "^test") {
	add_header X-Matched "yes";
}
add_header X-Always "present";
`,
			requestHeaders: map[string]string{
				"X-Custom": "other-value",
			},
			expectedResponseHeaders: map[string]string{
				"X-Matched": "",
				"X-Always":  "present",
			},
		},
		{
			desc: "if directive with negative case-insensitive regex match",
			configurationSnippet: `
if ($http_x_custom !~* "^admin") {
	add_header X-Not-Admin "true";
}
`,
			requestHeaders: map[string]string{
				"X-Custom": "user-request",
			},
			expectedResponseHeaders: map[string]string{
				"X-Not-Admin": "true",
			},
		},
		{
			desc: "if directive with negative case-insensitive regex match - should not match",
			configurationSnippet: `
if ($http_x_custom !~* "^admin") {
	add_header X-Not-Admin "true";
}
add_header X-Processed "yes";
`,
			requestHeaders: map[string]string{
				"X-Custom": "ADMIN-request",
			},
			expectedResponseHeaders: map[string]string{
				"X-Not-Admin": "",
				"X-Processed": "yes",
			},
		},
		{
			desc: "if directive with set variable check",
			configurationSnippet: `
set $flag "enabled";
if ($flag) {
	add_header X-Flag-Set "yes";
}
`,
			expectedResponseHeaders: map[string]string{
				"X-Flag-Set": "yes",
			},
		},
		{
			desc: "all directives combined",
			configurationSnippet: `
set $backend_type "api";
proxy_set_header X-Backend-Type $backend_type;
if ($request_method = GET) {
	add_header X-Read-Only "true";
	more_set_headers "X-Cache-Control:public";
}
add_header X-Powered-By "traefik";
`,
			method: http.MethodGet,
			expectedResponseHeaders: map[string]string{
				"X-Read-Only":     "true",
				"X-Cache-Control": "public",
			},
			expectedRequestHeaders: map[string]string{
				"X-Backend-Type": "api",
			},
		},
		{
			desc: "server and configuration snippets interaction",
			serverSnippet: `
add_header X-Server "server-value";
set $shared "from-server";
`,
			configurationSnippet: `
add_header X-Config "config-value";
`,
			expectedResponseHeaders: map[string]string{
				"X-Server": "",
				"X-Config": "config-value",
			},
		},
		{
			desc: "return directive with status code and text",
			configurationSnippet: `
return 403 "Forbidden";
`,
			expectedStatusCode: http.StatusForbidden,
			expectedBody:       "Forbidden",
		},
		{
			desc: "return directive with 200 status",
			configurationSnippet: `
return 200 "OK";
`,
			expectedStatusCode: http.StatusOK,
			expectedBody:       "OK",
		},
		{
			desc: "return directive inside if block - condition matches",
			configurationSnippet: `
if ($request_method = POST) {
	return 405 "Method Not Allowed";
}
add_header X-Allowed "true";
`,
			method:             http.MethodPost,
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedBody:       "Method Not Allowed",
		},
		{
			desc: "return directive inside if block - condition does not match",
			configurationSnippet: `
if ($request_method = POST) {
	return 405 "Method Not Allowed";
}
add_header X-Allowed "true";
`,
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
			expectedResponseHeaders: map[string]string{
				"X-Allowed": "true",
			},
		},
		{
			desc: "return directive doesn't stop processing headers",
			configurationSnippet: `
return 204 "";
add_header X-Should-Appear "value";
`,
			expectedStatusCode: http.StatusNoContent,
			expectedBody:       "",
			expectedResponseHeaders: map[string]string{
				"X-Should-Appear": "value",
			},
		},
		{
			desc: "location without return returns 503",
			serverSnippet: `
location /api {
	add_header X-Location "api";
}
`,
			path:               "/api/users",
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedResponseHeaders: map[string]string{
				"X-Location": "api",
			},
		},
		{
			desc: "location directive with prefix match - not matching continues to next",
			serverSnippet: `
location /api {
	return 200 "OK";
}
add_header X-Always "present";
`,
			path: "/web/users",
			expectedResponseHeaders: map[string]string{
				"X-Always": "present",
			},
		},
		{
			desc: "location directive with exact match and return",
			serverSnippet: `
location = /exact {
	return 200 "exact";
}
`,
			path:               "/exact",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "exact",
		},
		{
			desc: "location directive with exact match - not matching continues to next",
			serverSnippet: `
location = /exact {
	return 200 "exact";
}
add_header X-Always "present";
`,
			path: "/exact/more",
			expectedResponseHeaders: map[string]string{
				"X-Always": "present",
			},
		},
		{
			desc: "location directive with regex match and return",
			serverSnippet: `
location ~ ^/api/v[0-9]+/ {
	return 200 "versioned";
}
`,
			path:               "/api/v2/users",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "versioned",
		},
		{
			desc: "location directive with regex match - not matching continues to next",
			serverSnippet: `
location ~ ^/api/v[0-9]+/ {
	return 200 "versioned";
}
add_header X-Always "present";
`,
			path: "/api/latest/users",
			expectedResponseHeaders: map[string]string{
				"X-Always": "present",
			},
		},
		{
			desc: "location with return applies add_header from same block",
			serverSnippet: `
location /blocked {
	add_header X-Block-Header "block-value";
	return 403 "Blocked";
}
`,
			path:               "/blocked/path",
			expectedStatusCode: http.StatusForbidden,
			expectedBody:       "Blocked",
			expectedResponseHeaders: map[string]string{
				"X-Block-Header": "block-value",
			},
		},
		{
			desc: "location with return applies more_set_headers from same block",
			serverSnippet: `
location /blocked {
	more_set_headers "X-More-Header:more-value";
	return 403 "Blocked";
}
`,
			path:               "/blocked/path",
			expectedStatusCode: http.StatusForbidden,
			expectedBody:       "Blocked",
			expectedResponseHeaders: map[string]string{
				"X-More-Header": "more-value",
			},
		},
		{
			desc: "location with return applies both add_header and more_set_headers",
			serverSnippet: `
location /api {
	add_header X-Add "add-value";
	more_set_headers "X-More:more-value";
	return 200 "OK";
}
`,
			path:               "/api/endpoint",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "OK",
			expectedResponseHeaders: map[string]string{
				"X-Add":  "add-value",
				"X-More": "more-value",
			},
		},
		{
			desc: "add_header only applied in deepest block - location overrides root",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	return 200 "OK";
}
`,
			path:               "/api/endpoint",
			expectedStatusCode: http.StatusOK,
			expectedBody:       "OK",
			expectedResponseHeaders: map[string]string{
				"X-Level": "location",
			},
		},
		{
			desc: "add_header only applied in deepest block - nested if inside location",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	if ($request_method = GET) {
		add_header X-Level "if-block";
		return 200 "OK";
	}
}
`,
			path:               "/api/endpoint",
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
			expectedBody:       "OK",
			expectedResponseHeaders: map[string]string{
				"X-Level": "if-block",
			},
		},
		{
			desc: "add_header from location when if condition not matched",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	if ($request_method = POST) {
		add_header X-Level "if-block";
		return 200 "POST";
	}
	return 200 "OTHER";
}
`,
			path:               "/api/endpoint",
			method:             http.MethodGet,
			expectedStatusCode: http.StatusOK,
			expectedBody:       "OTHER",
			expectedResponseHeaders: map[string]string{
				"X-Level": "location",
			},
		},
		{
			desc: "root add_header applied when location not matched",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	return 200 "API";
}
`,
			path: "/web/endpoint",
			expectedResponseHeaders: map[string]string{
				"X-Level": "root",
			},
		},
		{
			desc: "more_set_input_headers sets request header",
			configurationSnippet: `
more_set_input_headers "X-Custom-Input:input-value";
`,
			expectedRequestHeaders: map[string]string{
				"X-Custom-Input": "input-value",
			},
		},
		{
			desc: "more_set_input_headers with variable interpolation",
			configurationSnippet: `
more_set_input_headers "X-Method-Input:$request_method";
`,
			expectedRequestHeaders: map[string]string{
				"X-Method-Input": "GET",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			ingressName := "directive-test-" + sanitizeName(test.desc)
			host := ingressName + "." + ingressControllerHost

			// Print ingress controller logs on failure
			t.Cleanup(func() {
				if t.Failed() {
					logs := getIngressControllerLogs(10)
					t.Logf("Last 10 lines of ingress controller logs:\n%s", logs)
				}
			})

			// Deploy ingress with snippet annotations
			err := deployIngress(ingressName, host, test.serverSnippet, test.configurationSnippet)
			require.NoError(t, err)

			// Cleanup ingress after test
			defer func() {
				_ = deleteIngress(ingressName)
			}()

			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			path := test.path
			if path == "" {
				path = "/test"
			}

			// Wait for ingress to be ready and make request
			resp, body := makeRequestWithRetryAndBody(t, host, method, path, test.requestHeaders, 20, 1000*time.Millisecond)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			expectedStatusCode := test.expectedStatusCode
			if expectedStatusCode == 0 {
				expectedStatusCode = http.StatusOK
			}
			assert.Equal(t, expectedStatusCode, resp.StatusCode)

			if test.expectedBody != "" {
				assert.Equal(t, test.expectedBody, body)
			}

			// Check response headers
			for header, expectedValue := range test.expectedResponseHeaders {
				actualExpected := expectedValue
				if test.dynamicHost {
					actualExpected = strings.ReplaceAll(expectedValue, "{{HOST}}", host)
				}
				assert.Equal(t, actualExpected, resp.Header.Get(header), "response header %s mismatch", header)
			}

			// Check request headers by parsing the whoami response body
			if len(test.expectedRequestHeaders) > 0 {
				requestHeaders := parseWhoamiHeaders(body)
				for header, expectedValue := range test.expectedRequestHeaders {
					actualExpected := expectedValue
					if test.dynamicHost {
						actualExpected = strings.ReplaceAll(expectedValue, "{{HOST}}", host)
					}
					assert.Equal(t, actualExpected, requestHeaders[header], "request header %s mismatch %s", header, body)
				}
			}
		})
	}
}

// makeRequestWithRetryAndBody makes an HTTP request with retries and returns both response and body.
func makeRequestWithRetryAndBody(t *testing.T, host, method, path string, headers map[string]string, maxRetries int, delay time.Duration) (*http.Response, string) {
	t.Helper()

	url := fmt.Sprintf("http://%s:%s%s", ingressControllerHost, ingressControllerPort, path)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		req.Host = host
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			lastErr = err
			time.Sleep(delay)
			continue
		}

		// For non-OK status codes that are expected (like 403, 405), return them
		// Only retry on 404 (ingress not ready) or connection errors
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode > http.StatusInternalServerError {
			resp.Body.Close()
			lastErr = fmt.Errorf("not found (ingress not ready)")
			time.Sleep(delay)
			continue
		}

		return resp, string(body)
	}

	t.Logf("request failed after %d retries: %v", maxRetries, lastErr)
	return nil, ""
}

// parseWhoamiHeaders parses the traefik/whoami response body and extracts headers.
// The whoami format includes lines like "Header: Value".
func parseWhoamiHeaders(body string) map[string]string {
	headers := make(map[string]string)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		// whoami format: "HeaderName: value"
		if idx := strings.Index(line, ": "); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+2:])
			headers[key] = value
		}
	}
	return headers
}

// getIngressControllerLogs retrieves the last N lines of the ingress controller logs.
func getIngressControllerLogs(lines int) string {
	// Get pods with the ingress controller label
	cmd := exec.Command("kubectl", "logs",
		"-l", "app.kubernetes.io/name=traefik",
		"-n", testNamespace,
		"--tail", fmt.Sprintf("%d", lines),
		"--all-containers=true",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("failed to get ingress controller logs: %v\noutput: %s", err, string(output))
	}
	return string(output)
}
