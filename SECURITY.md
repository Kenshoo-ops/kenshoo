# Security Policy

Thank you for helping keep kenshoo and its users safe.

## Supported versions

kenshoo is in early development and has no stable release yet. Security fixes are applied to the `main` branch. Once versioned releases exist, this section will list which versions receive fixes.

## Reporting a vulnerability

**Please do not report security vulnerabilities in public issues, pull requests or discussions.**

Report privately through GitHub:

1. Open [Report a vulnerability](https://github.com/Kenshoo-ops/kenshoo/security/advisories/new) in this repository's **Security** tab.
2. Describe the problem, the affected component, and steps to reproduce it.
3. Include the impact as you understand it, and a suggested fix if you have one.

## What to expect

This is a small, volunteer-maintained project, so the timelines below are best-effort goals rather than guarantees:

- **Acknowledgement** within 7 days.
- **Assessment** and an initial answer about severity and scope within 14 days.
- **Fix and release** as soon as practical, with the speed depending on severity.
- **Coordinated disclosure:** we ask that you give us up to 90 days to release a fix before you publish details. We will credit you in the advisory unless you prefer to stay anonymous.

## Scope

In scope for this repository:

- The verifier's code, including how it handles backup files, database credentials and storage credentials
- The container image and its build configuration
- Anything that could let the verifier read, modify or delete data it should not touch
- Unsafe handling of untrusted input, such as a malicious backup file or configuration

Out of scope:

- Vulnerabilities in third-party tools or services this project depends on. Please report those to their maintainers. We still want to hear about it if a dependency issue affects this project.
- Findings that need physical access, social engineering, or an already-compromised maintainer account.
- Automated scanner output without a demonstrated impact.

## Good-faith research

If you make a good-faith effort to follow this policy, avoid harming other people's data or systems, and give us a reasonable chance to fix the issue, we will not pursue action against you for your research.
