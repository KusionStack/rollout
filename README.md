# Rollout
Rollout is a general-purpose orchestration engine for release and operations. It addresses cross-cluster deployment and operations issues.

## Overview
![Rollout](docs/resources/rollout_arch.png)

## Features

## Documentation

## Contributing
Rollout is an open-source project, and we welcome contributions from the community. Please read this guide before submitting any pull requests.

### Guidelines for Contributing Code
To contribute code to Rollout, please follow these guidelines:

1. Fork the repository and create a new branch from **master**.
2. Write your code and tests.
3. Ensure that all tests pass by running `make test`.
4. Run `make` to format your code and ensure your code can be built successfully.
5. Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) to format your commit messages.
6. Submit a pull request to the **master** branch.

### Commit Convention
Rollout uses conventional commits to format commit messages. This helps us keep our commit history clean and organized.
The format of a conventional commit message is:
```bash
<type>[optional scope]: <description>

[optional body]

[optional footer]
```
- `<type>` is required and specifies the type of the commit, such as feat (new feature), fix (bug fix), or docs (documentation).
- `[optional scope]` is a short, specific context for the commit, such as a filename or module name.
- `<description>` is a brief summary of the changes.
- `[optional body]` provides more detailed information about the changes.
- `[optional footer]` includes any additional information, such as links to issues or pull requests.
Here are some examples of conventional commit messages:
```bash
feat(config): Add support for YAML configuration files

This commit adds a new YAML configuration parser to Rollout. The parser has been tested extensively and is now ready for use.

Closes #123
```

If you use Goland or Vscode, you can install the plugin or extension to help you format your commit messages.
- [Goland Conventional Commit](https://plugins.jetbrains.com/plugin/13389-conventional-commit)
- [VSCode Conventional Commit](https://marketplace.visualstudio.com/items?itemName=vivaxy.vscode-conventional-commits)

### Unit Tests
All contributions to Rollout must include appropriate unit tests.

To run all tests, use the `make test` command.




