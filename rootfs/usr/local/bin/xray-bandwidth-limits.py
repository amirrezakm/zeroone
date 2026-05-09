#!/usr/bin/env python3
import json
import subprocess
import sys
from pathlib import Path

CONFIG = Path("/usr/local/etc/xray/bandwidth-limits.json")
DEV = "eth0"
BASE_MARK = 41000
BASE_CLASS = 100
DEFAULT_RATE = "1000mbit"


def run(cmd, check=False, input_text=None):
    p = subprocess.run(cmd, text=True, input=input_text, capture_output=True)
    if check and p.returncode != 0:
        raise RuntimeError(f"{' '.join(cmd)} failed: {p.stderr or p.stdout}")
    return p


def load_limits():
    try:
        data = json.loads(CONFIG.read_text())
    except Exception:
        return {}
    return data if isinstance(data, dict) else {}


def active_limits():
    items = []
    for email, item in sorted(load_limits().items()):
        try:
            port = int(item.get("port"))
            down = float(item.get("download_mbps", 0))
            up = float(item.get("upload_mbps", 0))
        except Exception:
            continue
        if port > 0 and (down > 0 or up > 0):
            items.append({"email": email, "port": port, "download_mbps": down, "upload_mbps": up})
    return items


def nft_script(items):
    lines = [
        "table inet xray_bw {",
        "  chain output {",
        "    type route hook output priority mangle; policy accept;",
    ]
    for idx, item in enumerate(items, start=1):
        mark = BASE_MARK + idx
        lines.append(f"    tcp sport {item['port']} meta mark set {mark}")
    lines += ["  }", "}"]
    return "\n".join(lines) + "\n"


def clear_all():
    run(["nft", "delete", "table", "inet", "xray_bw"])
    run(["tc", "qdisc", "del", "dev", DEV, "root"])
    run(["tc", "qdisc", "del", "dev", DEV, "clsact"])
    run(["tc", "qdisc", "replace", "dev", DEV, "root", "fq"])


def apply_limits():
    items = active_limits()
    if not items:
        clear_all()
        return
    run(["nft", "delete", "table", "inet", "xray_bw"])
    run(["nft", "-f", "-"], check=True, input_text=nft_script(items))
    run(["tc", "qdisc", "replace", "dev", DEV, "root", "handle", "1:", "htb", "default", "999"], check=True)
    run(["tc", "class", "replace", "dev", DEV, "parent", "1:", "classid", "1:1", "htb", "rate", DEFAULT_RATE, "ceil", DEFAULT_RATE], check=True)
    run(["tc", "class", "replace", "dev", DEV, "parent", "1:1", "classid", "1:999", "htb", "rate", DEFAULT_RATE, "ceil", DEFAULT_RATE], check=True)
    run(["tc", "qdisc", "replace", "dev", DEV, "parent", "1:999", "fq"], check=True)
    run(["tc", "qdisc", "replace", "dev", DEV, "clsact"], check=True)
    for idx, item in enumerate(items, start=1):
        mark = BASE_MARK + idx
        class_num = BASE_CLASS + idx
        classid = f"1:{class_num}"
        port = str(item["port"])
        down = item["download_mbps"]
        up = item["upload_mbps"]
        if down > 0:
            rate = f"{down}mbit"
            run(["tc", "class", "replace", "dev", DEV, "parent", "1:1", "classid", classid, "htb", "rate", rate, "ceil", rate], check=True)
            run(["tc", "qdisc", "replace", "dev", DEV, "parent", classid, "fq"], check=True)
            run(["tc", "filter", "replace", "dev", DEV, "parent", "1:", "protocol", "ip", "prio", str(100 + idx), "handle", str(mark), "fw", "flowid", classid], check=True)
        if up > 0:
            rate = f"{up}mbit"
            burst = f"{max(64, int(up * 128))}k"
            run([
                "tc", "filter", "replace", "dev", DEV, "ingress", "protocol", "ip", "prio", str(100 + idx),
                "flower", "ip_proto", "tcp", "dst_port", port,
                "action", "police", "rate", rate, "burst", burst, "conform-exceed", "drop"
            ], check=True)


def status():
    items = active_limits()
    print(json.dumps({
        "active_limits": items,
        "tc_root": run(["tc", "qdisc", "show", "dev", DEV]).stdout.strip(),
        "tc_ingress": run(["tc", "filter", "show", "dev", DEV, "ingress"]).stdout.strip(),
    }, indent=2))


def main():
    cmd = sys.argv[1] if len(sys.argv) > 1 else "apply"
    if cmd == "apply":
        apply_limits()
    elif cmd == "clear":
        clear_all()
    elif cmd == "status":
        status()
    else:
        raise SystemExit("usage: xray-bandwidth-limits.py [apply|clear|status]")


if __name__ == "__main__":
    main()
