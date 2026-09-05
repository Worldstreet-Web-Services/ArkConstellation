#!/usr/bin/env python3
"""ArkConstellation devnet faucet - stdlib only, wraps `arkd tx bank send`."""
import json
import os
import re
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

CHAIN_ID = os.environ.get("CHAIN_ID", "arkdevnet_9000-1")
NODE = os.environ.get("NODE", "http://sentry-0:26657")
HOME_DIR = os.environ.get("HOME_DIR", "/home/nonroot/.ark/faucet")
KEY_NAME = os.environ.get("KEY_NAME", "faucet")
DENOM = os.environ.get("DENOM", "esp")
HRP = os.environ.get("BECH32_HRP", "ark")
AMOUNT_KASH = int(os.environ.get("AMOUNT_KASH", "10"))
DAILY_CAP_KASH = int(os.environ.get("DAILY_CAP_KASH", "50"))
AMOUNT_ESP = AMOUNT_KASH * 10**18
DAILY_CAP_ESP = DAILY_CAP_KASH * 10**18
STATE_FILE = os.environ.get("STATE_FILE", "/home/nonroot/.ark/faucet/state.json")
PORT = int(os.environ.get("PORT", "8088"))

lock = threading.Lock()

# --- bech32 (BIP-0173 reference implementation, encode-only) ---
CHARSET = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"


def bech32_polymod(values):
    generator = [0x3B6A57B2, 0x26508E6D, 0x1EA119FA, 0x3D4233DD, 0x2A1462B3]
    chk = 1
    for value in values:
        top = chk >> 25
        chk = (chk & 0x1FFFFFF) << 5 ^ value
        for i in range(5):
            chk ^= generator[i] if ((top >> i) & 1) else 0
    return chk


def bech32_hrp_expand(hrp):
    return [ord(x) >> 5 for x in hrp] + [0] + [ord(x) & 31 for x in hrp]


def bech32_create_checksum(hrp, data):
    values = bech32_hrp_expand(hrp) + data
    polymod = bech32_polymod(values + [0, 0, 0, 0, 0, 0]) ^ 1
    return [(polymod >> 5 * (5 - i)) & 31 for i in range(6)]


def bech32_encode(hrp, data):
    combined = data + bech32_create_checksum(hrp, data)
    return hrp + "1" + "".join(CHARSET[d] for d in combined)


def convertbits(data, frombits, tobits, pad=True):
    acc, bits, ret = 0, 0, []
    maxv = (1 << tobits) - 1
    for value in data:
        acc = (acc << frombits) | value
        bits += frombits
        while bits >= tobits:
            bits -= tobits
            ret.append((acc >> bits) & maxv)
    if pad and bits:
        ret.append((acc << (tobits - bits)) & maxv)
    return ret


def eth_hex_to_bech32(hexaddr):
    raw = bytes.fromhex(hexaddr.lower().replace("0x", ""))
    return bech32_encode(HRP, convertbits(list(raw), 8, 5))


ADDR_0X_RE = re.compile(r"^0x[0-9a-fA-F]{40}$")
ADDR_BECH32_RE = re.compile(r"^" + HRP + r"1[0-9a-z]{38}$")


def normalize_address(raw):
    raw = raw.strip()
    if ADDR_0X_RE.match(raw):
        return eth_hex_to_bech32(raw)
    if ADDR_BECH32_RE.match(raw):
        return raw
    return None


# --- rate-limit state ---
def load_state():
    try:
        with open(STATE_FILE) as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return {}


def save_state(state):
    os.makedirs(os.path.dirname(STATE_FILE), exist_ok=True)
    with open(STATE_FILE, "w") as f:
        json.dump(state, f)


def prune(entries, now):
    return [e for e in entries if now - e["ts"] < 86400]


class FaucetError(Exception):
    pass


def current_gas_price():
    """This chain prices even native bank-send txs off the EVM fee market's
    dynamic base fee (not a small fixed value), so a hardcoded --gas-prices
    gets rejected as soon as the base fee drifts above it."""
    result = subprocess.run(
        ["arkd", "query", "feemarket", "base-fee", "--node", NODE, "-o", "json"],
        capture_output=True, text=True, timeout=10,
    )
    try:
        base_fee = float(json.loads(result.stdout)["base_fee"])
    except (json.JSONDecodeError, KeyError, ValueError):
        raise FaucetError(result.stderr.strip() or "failed to query current gas price")
    return int(base_fee * 1.5) + 1


def broadcast_and_confirm(addr):
    cmd = [
        "arkd", "tx", "bank", "send", KEY_NAME, addr, f"{AMOUNT_ESP}{DENOM}",
        "--home", HOME_DIR, "--keyring-backend", "test",
        "--chain-id", CHAIN_ID, "--node", NODE,
        "--gas", "auto", "--gas-adjustment", "1.3",
        "--gas-prices", f"{current_gas_price()}{DENOM}",
        "--broadcast-mode", "sync", "--yes", "-o", "json",
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        raise FaucetError(result.stderr.strip() or "broadcast failed")
    if data.get("code", 1) != 0:
        raise FaucetError(data.get("raw_log") or "broadcast rejected")
    txhash = data["txhash"]

    for _ in range(15):
        time.sleep(1)
        q = subprocess.run(
            ["arkd", "query", "tx", txhash, "--node", NODE, "-o", "json"],
            capture_output=True, text=True,
        )
        if q.returncode == 0:
            qdata = json.loads(q.stdout)
            if qdata.get("code", 1) != 0:
                raise FaucetError(qdata.get("raw_log") or "tx failed on-chain")
            return txhash
    raise FaucetError("timed out waiting for on-chain confirmation")


FORM_HTML = """<!doctype html>
<html><head><title>ArkConstellation Devnet Faucet</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{{font-family:system-ui,sans-serif;max-width:520px;margin:60px auto;padding:0 20px;color:#1a1a1a}}
input{{width:100%;padding:10px;font-size:15px;box-sizing:border-box;margin:8px 0}}
button{{padding:10px 20px;font-size:15px;cursor:pointer}}
#result{{margin-top:16px;padding:12px;border-radius:6px;display:none;font-size:14px;word-break:break-all}}
.ok{{background:#e6f7ec;border:1px solid #34a853}}
.err{{background:#fdecea;border:1px solid #ea4335}}
code{{background:#f0f0f0;padding:2px 5px;border-radius:3px}}
</style></head>
<body>
<h2>ArkConstellation Devnet Faucet</h2>
<p>{amount} KASH per request, {cap} KASH max per address per 24h. Accepts <code>{hrp}1...</code> or <code>0x...</code> addresses.</p>
<input id="addr" placeholder="ark1... or 0x...">
<button onclick="send()">Request {amount} KASH</button>
<div id="result"></div>
<script>
async function send() {{
  const addr = document.getElementById('addr').value;
  const box = document.getElementById('result');
  box.style.display = 'block';
  box.className = ''; box.textContent = 'Sending...';
  try {{
    const r = await fetch('/faucet', {{method:'POST', headers:{{'Content-Type':'application/json'}}, body: JSON.stringify({{address: addr}})}});
    const j = await r.json();
    if (r.ok) {{ box.className = 'ok'; box.textContent = `Sent! tx: ${{j.tx_hash}}`; }}
    else {{ box.className = 'err'; box.textContent = j.error; }}
  }} catch (e) {{ box.className = 'err'; box.textContent = String(e); }}
}}
</script>
</body></html>"""


class Handler(BaseHTTPRequestHandler):
    def _json(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type")
        self.end_headers()

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"status": "ok"})
            return
        body = FORM_HTML.format(amount=AMOUNT_KASH, cap=DAILY_CAP_KASH, hrp=HRP).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path != "/faucet":
            self._json(404, {"error": "not found"})
            return
        length = int(self.headers.get("Content-Length", 0))
        try:
            payload = json.loads(self.rfile.read(length) or b"{}")
            raw_addr = payload["address"]
        except (json.JSONDecodeError, KeyError):
            self._json(400, {"error": "expected JSON body: {\"address\": \"...\"}"})
            return

        addr = normalize_address(raw_addr)
        if not addr:
            self._json(400, {"error": "invalid address - expected ark1... or 0x... (40 hex chars)"})
            return

        with lock:
            now = time.time()
            state = load_state()
            entries = prune(state.get(addr, []), now)
            used = sum(e["amount"] for e in entries)
            if used + AMOUNT_ESP > DAILY_CAP_ESP:
                reset_in = int(86400 - (now - entries[0]["ts"])) if entries else 86400
                self._json(429, {
                    "error": f"daily cap of {DAILY_CAP_KASH} KASH reached for this address",
                    "retry_after_seconds": reset_in,
                })
                return
            try:
                txhash = broadcast_and_confirm(addr)
            except FaucetError as e:
                self._json(502, {"error": str(e)})
                return
            entries.append({"ts": now, "amount": AMOUNT_ESP})
            state[addr] = entries
            save_state(state)

        self._json(200, {
            "address": addr,
            "amount_kash": AMOUNT_KASH,
            "tx_hash": txhash,
        })

    def log_message(self, fmt, *args):
        print(f"[faucet] {self.address_string()} - {fmt % args}")


if __name__ == "__main__":
    print(f"[faucet] listening on :{PORT}, {AMOUNT_KASH} KASH/req, {DAILY_CAP_KASH} KASH/24h cap")
    HTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
