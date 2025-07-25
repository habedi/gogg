# Contributing to Gogg

Thank you for considering contributing to developing Gogg!
Your contributions help improve the project and make it more useful for everyone.

## How to Contribute

### Reporting Bugs

1. Open an issue on the [issue tracker](https://github.com/habedi/gogg/issues).
2. Include information like steps to reproduce, expected/actual behaviour, and relevant logs or screenshots.

### Suggesting Features

1. Open an issue on the [issue tracker](https://github.com/habedi/gogg/issues).
2. Write a little about the feature, its purpose, and potential implementation ideas.

## Submitting Pull Requests

- Ensure all tests pass before submitting a pull request.
- Write a clear description of the changes you made and the reasons behind them.

> [!IMPORTANT]
> It's assumed that by submitting a pull request, you agree to license your contributions under the project's license.

## Development Workflow

### Prerequisites

Install system dependencies (Go and GNU Make).

```shell
sudo apt-get install -y golang-go make
```

- Use the `make install-deps` command to install the development dependencies.
- Use the `make setup-hooks` command to set up Git hooks for pre-commit checks.

### Code Style

- Use the `make format` command to format the code.

### Running Tests

- Use the `make test` command to run the tests.

### Running Linters

- Use the `make lint` command to run the linters.

### See Available Commands

- Run `make help` to see all available commands for managing different tasks.

## Code of Conduct

We adhere to the [Go Community Code of Conduct](https://go.dev/conduct).
