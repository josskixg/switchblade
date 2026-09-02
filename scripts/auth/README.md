# Browser-Auth Login Scripts

Switchblade does **not** bundle browser-login bots. If you want providers that
require browser-based OAuth (canva, qoder, …) to re-authenticate themselves,
place one Python script per provider in this directory following the contract
below. Accounts whose provider has no script can still be added manually via
`POST /api/accounts` with tokens pasted in.

## Script Contract

`internal/auth/queue.go` spawns each script as:

```
<python> scripts/<provider>_login.py --email <email> --password <password>
```

- **Provider name** comes from the account's `provider` column — the file must
  be named exactly `<provider>_login.py`, e.g. `canva_login.py`.
- The Python executable defaults to `python3`; override with the
  `PYTHON_PATH` environment variable (`SetPythonPath` in `queue.go`).
- **Timeout:** 120 seconds per run — the script must finish a full login
  (including browser automation and any 2FA pause) inside that window.
- **On success:** exit `0` and print the token payload to **stdout** (plain
  text, trailing whitespace is trimmed). Whatever is printed is stored in the
  account's `tokens` column — JSON with `access_token`/`refresh_token` is the
  convention providers expect.
- **On failure:** non-zero exit; stderr is captured verbatim into the
  account's `error_message`.

## Example Skeleton

```python
#!/usr/bin/env python3
"""Login bot for the <provider> provider. See scripts/auth/README.md."""
import argparse
import sys


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--email", required=True)
    parser.add_argument("--password", required=True)
    args = parser.parse_args()

    try:
        tokens = do_browser_login(args.email, args.password)  # your Playwright/CDP flow
    except Exception as exc:  # stderr becomes the account's error_message
        print(f"login failed: {exc}", file=sys.stderr)
        return 1

    print(tokens)  # stdout -> accounts.tokens
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

## Notes

- `HEADLESS=true` (default) is a Switchblade setting; pass it through to your
  own automation if you honor it — the queue does not forward it.
- Keep credentials out of logs; the password arrives via argv, which is
  visible in the local process list — treat the host as trusted.
