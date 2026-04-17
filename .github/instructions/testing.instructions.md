---
applyTo: "**/*_test.go"
description: "Use when writing or modifying Go test files. Covers table-driven tests, K8s client mocking, httptest patterns, and file I/O testing."
---
# Testing Guidelines

## Test Structure

- Use table-driven tests with `t.Run(name, ...)` subtests.
- Use standard library only (`testing`, `net/http/httptest`). No testify or other assertion libraries.
- Name test functions `TestXxx_MethodName_scenario`.
- Use `t.Helper()` in shared assertion helpers.

## Mocking External Dependencies

**Kubernetes clients:** Use `k8s.io/client-go/kubernetes/fake` and `k8s.io/client-go/dynamic/fake` with `discovery.NewFromClients()`.

```go
dynClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
clientset := fake.NewSimpleClientset(secrets...)
d := discovery.NewFromClients(dynClient, clientset, "")
```

**pgAdmin HTTP API:** Use `httptest.NewServer` to simulate pgAdmin responses. Do not wrap `http.Client` in an interface just for tests.

**File I/O:** Use `t.TempDir()` for atomic write tests. Pass temp paths via config — never write to real `/shared/`.

**Environment variables:** Use `t.Setenv()` for config tests.

## Pure Function Tests

Functions like `GenerateServersJSON`, `GeneratePgpass`, `escapePgpass`, `ServerKey`, and `extractCSRFToken` are pure — test them with table-driven cases, no setup needed.

## Assertions

Compare with `==`, `!=`, `errors.Is`, or `reflect.DeepEqual`. Use `t.Errorf`/`t.Fatalf` for failures:

```go
if got != want {
    t.Errorf("ServerKey() = %q, want %q", got, want)
}
```

For JSON output, unmarshal both and compare structs rather than comparing raw strings.
