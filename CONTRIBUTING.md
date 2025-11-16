# How to Contribute (WIP)

All contributions are welcome. Below you'll find guidelines on how you can help.

## Table of Contents
- [Reporting Bugs](#reporting-bugs)
- [Suggesting Enhancements](#suggesting-enhancements)
- [Your First Code Contribution](#your-first-code-contribution)
- [Pull Request Process](#pull-request-process)

## Reporting Bugs

If you find a bug, please report it in the [Issues](https://github.com/Niutaq/Gix/issues) section. A good bug report should include:

-   **Title:** Short and descriptive.
-   **Description:** A detailed explanation of the problem.
-   **Steps to reproduce:** The simplest possible instructions on how to reproduce the bug.
-   **Expected behavior:** What should have happened.
-   **Actual behavior:** What actually happened.
-   **Environment:** Application version, operating system.

## Suggesting Enhancements

Have an idea for a new feature or an improvement to an existing one? Open an issue in the [Issues](https://github.com/Niutaq/Gix/issues) section and describe your idea. Explain what problem it solves and why you think it would be a valuable addition.

## Your First Code Contribution

Want to write some code? Great! Here's how to get started:

1.  **Fork the repository:** Click the "Fork" button in the top right corner of this page.
2.  **Clone your fork:**
    ```bash
    git clone https://github.com/YOUR_USERNAME/Gix.git
    ```
3.  **Create a new branch:**
    ```bash
    git checkout -b your-feature-or-fix-name
    ```
4.  **Make your changes:** Write code and tests for your functionality.
5.  **Run tests and linter:** Make sure everything works and the code is properly formatted. The project uses `golangci-lint` to check code quality.
    ```bash
    go build ./...
    golangci-lint run
    ```
6.  **Commit your changes:** Use descriptive commit messages. We suggest using [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) convention.
    ```bash
    git commit -m "feat: Add new feature"
    ```
7.  **Push your changes to your fork:**
    ```bash
    git push origin your-feature-or-fix-name
    ```
8.  **Open a Pull Request:** Go to the original Gix repository and open a new Pull Request.

## Pull Request Process

1.  **CI Checks:** After opening a Pull Request, tests and linter will automatically run on GitHub Actions. All checks must pass successfully.
2.  **Code Review:** One of the project maintainers will review your changes, may ask questions, or request revisions.
3.  **Merge:** Once approved, your Pull Request will be merged into the main branch of the project.

Thank you for your contribution!
