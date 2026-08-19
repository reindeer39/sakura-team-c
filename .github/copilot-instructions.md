# Copilot Code Review Instructions

This repository is a short-term team development project using Go, Docker, and Terraform.

When reviewing pull requests, prioritize correctness, security, maintainability, and reliability.

## General

- Identify potential bugs and regressions.
- Point out unclear or unnecessarily complex code.
- Check whether error cases are handled correctly.
- Check whether new or changed behavior should have tests.
- Identify duplicated code where appropriate.
- Prefer concrete and actionable review comments.
- Do not suggest large refactoring unless it is necessary for the change.
- Do not approve code merely because it compiles.

## Security

- Check for committed passwords, API keys, tokens, SSH keys, and other secrets.
- Check for hard-coded credentials and sensitive configuration.
- Check whether environment-specific configuration should use environment variables.
- Check input validation and unsafe user-controlled input.
- Check authentication and authorization where relevant.
- Point out unnecessarily exposed network ports or services.

## Go

For files under `app/backend`:

- Prefer idiomatic Go.
- Check all important returned errors.
- Identify ignored errors that may cause incorrect behavior.
- Check HTTP handlers for input validation and appropriate status codes.
- Check resource cleanup such as `defer rows.Close()` and response body closing.
- Check for goroutine leaks, race conditions, and unsafe shared state.
- Check context cancellation and timeout handling where relevant.
- Recommend tests for new handlers, services, and business logic.
- Prefer simple and readable code over unnecessary abstractions.

## Tests

- Check whether tests cover the behavior changed by the pull request.
- Include failure cases and boundary conditions where appropriate.
- Avoid tests that depend unnecessarily on external services.
- Prefer deterministic tests.
- Point out tests that only verify implementation details instead of behavior.

## Docker

- Check that secrets are not copied into container images.
- Check for unnecessary privileges.
- Check whether unnecessary files are included in the image.
- Check whether the image can be made smaller or safer without excessive complexity.
- Check whether container configuration is reproducible.

## Terraform

For files under `infra/terraform`:

- Pay special attention to destructive infrastructure changes.
- Identify resources that may unexpectedly be replaced or deleted.
- Check for hard-coded credentials, IP addresses, and environment-specific values.
- Prefer Terraform variables for configurable values.
- Check whether sensitive values are handled appropriately.
- Check network exposure and firewall configuration.
- Do not recommend `terraform apply` automatically unless the change is clearly safe.
- Point out any change that may affect Terraform state.

## Pull Request Scope

- Check whether the pull request matches its stated purpose.
- Point out unrelated changes.
- Prefer small, focused pull requests.
- If an issue is referenced, check whether the implementation appears to satisfy its completion conditions.
