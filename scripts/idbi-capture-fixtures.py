#!/usr/bin/env python3
"""
Capture real IDBI sandbox response shapes.

Reads every OpenAPI spec in docs/idbireposne/, pulls the seeded sample
request(s) out of each, fires them at the sandbox gateway through the local
SOCKS proxy, and writes {request, status, response} per call into
internal/provider/idbi/testdata/.

Prereq: a SOCKS proxy on 127.0.0.1:1080, i.e. the dev tunnel is up:
    ssh -D 1080 -N idbi-egress        (SSM ProxyCommand form)
  or, until the new instance exists:
    ssh -i ~/.ssh/zeyro-idbi.pem -D 1080 -N ec2-user@43.205.52.148

Usage:
    python scripts/idbi-capture-fixtures.py [--port 1080] [--timeout 30]
"""
import argparse
import glob
import json
import os
import sys
import time

import requests
import yaml

SPEC_DIR = os.path.join("docs", "idbireposne")
OUT_DIR = os.path.join("internal", "provider", "idbi", "testdata")
DEFAULT_BASE = "https://sandboxpocgatewayprod.idbi.bank.in"


def load_examples(spec):
    """Yield (method, url, path, example_name, body) for one spec dict."""
    servers = spec.get("servers") or [{}]
    base = (servers[0].get("url") or DEFAULT_BASE).rstrip("/")
    for path, ops in (spec.get("paths") or {}).items():
        if not isinstance(ops, dict):
            continue
        for method, op in ops.items():
            if method.lower() not in ("get", "post", "put", "patch", "delete"):
                continue
            if not isinstance(op, dict):
                continue
            rb = (((op.get("requestBody") or {}).get("content") or {})
                  .get("application/json") or {})
            examples = rb.get("examples") or {}
            if examples:
                for exn, exv in examples.items():
                    val = exv.get("value") if isinstance(exv, dict) else None
                    if val is None:
                        continue
                    if isinstance(val, str):
                        try:
                            val = json.loads(val)
                        except Exception:
                            pass
                    yield method.upper(), base + path, path, exn, val
            elif "example" in rb:
                val = rb["example"]
                if isinstance(val, str):
                    try:
                        val = json.loads(val)
                    except Exception:
                        pass
                yield method.upper(), base + path, path, "example", val
            else:
                # no body example - still worth a call to see the error shape
                yield method.upper(), base + path, path, "noexample", None


def slug(path, exn):
    name = path.strip("/").replace("/", "_")
    if exn and exn not in ("example", "sample 1"):
        name += "__" + exn.replace(" ", "")
    return name


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=1080)
    ap.add_argument("--timeout", type=int, default=30)
    ap.add_argument("--base", default=None,
                    help="override gateway base URL")
    args = ap.parse_args()

    proxy = f"socks5h://127.0.0.1:{args.port}"
    proxies = {"http": proxy, "https": proxy}

    # sanity: egress IP must be the whitelisted one
    try:
        ip = requests.get("https://checkip.amazonaws.com",
                          proxies=proxies, timeout=15).text.strip()
        print(f"egress IP via proxy: {ip}")
    except Exception as e:
        print(f"!! proxy not reachable on :{args.port} - is the tunnel up? ({e})")
        sys.exit(1)

    os.makedirs(OUT_DIR, exist_ok=True)
    specs = sorted(glob.glob(os.path.join(SPEC_DIR, "*.yaml")))
    seen = set()
    rows = []

    for f in specs:
        try:
            spec = yaml.safe_load(open(f, encoding="utf-8"))
        except Exception as e:
            print(f"skip {f}: {e}")
            continue
        if not isinstance(spec, dict):
            continue
        for method, url, path, exn, body in load_examples(spec):
            if args.base:
                url = args.base.rstrip("/") + path
            key = (path, json.dumps(body, sort_keys=True) if body else exn)
            if key in seen:
                continue
            seen.add(key)

            rec = {"spec_file": os.path.basename(f), "method": method,
                   "url": url, "path": path, "example": exn,
                   "request": body}
            try:
                r = requests.request(method, url, json=body, proxies=proxies,
                                     timeout=args.timeout,
                                     headers={"Content-Type": "application/json"})
                rec["status"] = r.status_code
                try:
                    rec["response"] = r.json()
                except Exception:
                    rec["response_text"] = r.text[:20000]
            except Exception as e:
                rec["error"] = str(e)
                rec["status"] = None

            out = os.path.join(OUT_DIR, slug(path, exn) + ".json")
            with open(out, "w", encoding="utf-8") as fh:
                json.dump(rec, fh, indent=2, ensure_ascii=False)
            rows.append((rec.get("status"), path, exn, os.path.basename(out)))
            print(f"  {str(rec.get('status')):>4}  {path}  ({exn})")
            time.sleep(0.4)

    print(f"\n{len(rows)} calls -> {OUT_DIR}\n")
    ok = sum(1 for s, *_ in rows if s == 200)
    print(f"200 OK: {ok}   other: {len(rows) - ok}")
    bad = [r for r in rows if r[0] != 200]
    if bad:
        print("\nnon-200:")
        for s, path, exn, fn in bad:
            print(f"  {str(s):>4}  {path}  ({exn})")


if __name__ == "__main__":
    main()
