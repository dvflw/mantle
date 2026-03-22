# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Mantle, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, email **security@dvflw.dev** with:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Triage**: Within 7 business days
- **Fix**: Depends on severity, typically within 30 days for critical issues

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | Yes       |

## Disclosure Policy

We follow coordinated disclosure with a 90-day window. After reporting:

1. We acknowledge receipt and begin investigation
2. We develop and test a fix
3. We release the fix and publish an advisory
4. You may disclose publicly after the fix is released, or after 90 days, whichever comes first

## Scope

The following areas are in scope for security reports:

- Authentication bypass (API key, OIDC)
- Cross-tenant data access or modification
- Credential exposure or decryption
- Injection vulnerabilities (SQL, CEL expression, JSON)
- Unauthorized workflow execution or cancellation
- Secret exfiltration through expressions or logs
- Privilege escalation in RBAC

## Out of Scope

- Denial of service through resource exhaustion (please report, but these are lower priority)
- Issues in dependencies (report upstream; let us know if it affects Mantle)
- Social engineering
