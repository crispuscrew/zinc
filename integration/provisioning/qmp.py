"""Speak real QMP over a private Unix socket; record device/backend observations."""

import json
import socket


class QMP:
    def __init__(self, path):
        self.socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.socket.settimeout(5)
        try:
            self.socket.connect(str(path))
            self.stream = self.socket.makefile("rwb")
            greeting = json.loads(self.stream.readline())
            assert "QMP" in greeting, greeting
            self.query("qmp_capabilities")
        except BaseException:
            self.close()
            raise

    def query(self, command, arguments=None):
        request = {"execute": command}
        if arguments is not None:
            request["arguments"] = arguments
        self.stream.write(json.dumps(request).encode() + b"\n")
        self.stream.flush()
        while True:
            line = self.stream.readline()
            if not line:
                raise RuntimeError("QMP closed before responding to " + command)
            response = json.loads(line)
            if "error" in response:
                raise RuntimeError(f"QMP {command}: {response['error']}")
            if "return" in response:
                return response["return"]

    def monitor(self, command):
        return self.query("human-monitor-command", {"command-line": command})

    def close(self):
        if hasattr(self, "stream"):
            self.stream.close()
        self.socket.close()

    def __enter__(self):
        return self

    def __exit__(self, *unused):
        self.close()
