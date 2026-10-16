#!/usr/bin/env python3
"""Read-only LiteLLM public-edge auth diagnostic. No model calls, no secrets printed."""
import json
import os
import re
import sys
import urllib.error
import urllib.request

BASE = "https://webhook.mujeeb24.com"
KEY = os.environ.get("LITELLM_API_KEY", "")
MARKERS = (
    "invalid", "expired", "forbidden", "model", "route", "permission", "access",
    "team", "org", "user", "proxy", "not allowed", "not authorized", "auth",
    "denied", "blocked", "cloudflare", "policy", "key", "budget",
)


def classify_error(raw, content_type):
    if "json" not in content_type:
        return {"format": "not-json"}
    try:
        obj = json.loads(raw)
    except (UnicodeError, ValueError):
        return {"format": "invalid-json"}
    if not isinstance(obj, dict):
        return {"format": type(obj).__name__}
    fields = sorted(k for k in obj if re.fullmatch(r"[a-zA-Z0-9_]{1,40}", k))
    error = obj.get("error")
    data = error if isinstance(error, dict) else obj
    def safe_label(key):
        value = data.get(key)
        return value if isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9_.:-]{1,48}", value) else "not-specified"
    # We output only known category booleans, never arbitrary raw error messages.
    raw_text = str(obj).lower()
    markers = [word.replace(" ", "_") for word in MARKERS if word in raw_text]
    def trusted_cloudflare_field(key):
        value = obj.get(key)
        if key in ("cloudflare_error", "owner_action_required", "retryable"):
            return value if isinstance(value, bool) else None
        if key == "status":
            return value if isinstance(value, int) else None
        value = str(value or "")
        if key == "ray_id":
            return value if re.fullmatch(r"[0-9a-fA-F]{8,32}(?:-[A-Za-z]{3})?", value) else "not-specified"
        return value if re.fullmatch(r"[A-Za-z0-9_.:-]{1,80}", value) else "not-specified"
    return {
        "format": "json",
        "root_keys": fields,
        "error_shape": type(error).__name__,
        "error_type": safe_label("type"),
        "error_code": safe_label("code"),
        "error_categories": markers,
        "cloudflare_details": {
            field: trusted_cloudflare_field(field) for field in (
                "cloudflare_error", "error_category", "error_name", "error_code",
                "owner_action_required", "retryable", "status", "ray_id"
            )
        },
    }


def probe(authenticated):
    request = urllib.request.Request(BASE + "/v1/models", method="GET")
    request.add_header("Accept", "application/json")
    if authenticated:
        request.add_header("Authorization", "Bearer " + KEY)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            raw = response.read(8192)
            status, headers = response.status, response.headers
    except urllib.error.HTTPError as exc:
        status, headers = exc.code, exc.headers
        raw = exc.read(8192)
    except urllib.error.URLError as exc:
        print("HTTPS connection failed:", type(exc.reason).__name__)
        return None
    ctype = headers.get("Content-Type", "").split(";")[0].lower()
    server = headers.get("Server", "")
    server = server if re.fullmatch(r"[a-zA-Z0-9_. -]{1,60}", server) else "not-specified"
    print(json.dumps({
        "probe": "with_virtual_key" if authenticated else "without_virtual_key",
        "status": status,
        "content_type": ctype[:60],
        "server_header": server,
        "cloudflare_ray_header_present": bool(headers.get("cf-ray")),
        "cloudflare_challenge": headers.get("cf-mitigated", "") == "challenge",
        "error": classify_error(raw, ctype) if status >= 400 else {"format": "success"},
    }, sort_keys=True))
    return status


if __name__ == "__main__":
    if not KEY:
        print("ERROR: LITELLM_API_KEY is unavailable. No request made.")
        sys.exit(1)
    anonymous = probe(False)
    authenticated = probe(True)
    if anonymous is None or authenticated is None:
        sys.exit(1)
    if authenticated == 200:
        print("SUCCESS: virtual key is authorized for model listing")
    else:
        print("DIAGNOSTIC ONLY: no live model completion attempted; inspect status and safe categories")
