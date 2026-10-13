"""Probe management ports while a link is failed: how long each target was
unreachable, and whether it came back on its own.

Runs on the control node — in the emulation, inside the network namespace
of the server that stands in for it — like compare.py, so the probe takes
the path the model describes. Every target is tried every 0.25 s with a
0.5 s connect timeout for the given number of seconds; the result is the
total time each target was unreachable, its longest outage, and the state
at the end.

    failover.py <seconds> <ip:port> [<ip:port> ...]
"""

import json
import socket
import sys
import time

INTERVAL = 0.25
TIMEOUT = 0.5


def up(host, port):
    try:
        with socket.create_connection((host, port), timeout=TIMEOUT):
            return True
    except OSError:
        return False


def main(seconds, targets):
    start = time.monotonic()
    state = {t: {"down_total": 0.0, "longest": 0.0, "down_since": None, "events": []} for t in targets}
    while time.monotonic() - start < seconds:
        tick = time.monotonic()
        for t in targets:
            host, port = t.rsplit(":", 1)
            ok = up(host, int(port))
            s = state[t]
            now = round(time.monotonic() - start, 2)
            if not ok and s["down_since"] is None:
                s["down_since"] = now
                s["events"].append([now, "down"])
            elif ok and s["down_since"] is not None:
                span = now - s["down_since"]
                s["down_total"] += span
                s["longest"] = max(s["longest"], span)
                s["down_since"] = None
                s["events"].append([now, "up"])
        time.sleep(max(0.0, INTERVAL - (time.monotonic() - tick)))
    end = round(time.monotonic() - start, 2)
    out = {}
    for t, s in state.items():
        if s["down_since"] is not None:
            span = end - s["down_since"]
            s["down_total"] += span
            s["longest"] = max(s["longest"], span)
        out[t] = {"down_total_s": round(s["down_total"], 2), "longest_outage_s": round(s["longest"], 2),
                  "up_at_end": s["down_since"] is None, "events": s["events"]}
    json.dump(out, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit("використання: failover.py <секунд> <ip:порт> [<ip:порт> ...]")
    main(float(sys.argv[1]), sys.argv[2:])
