#!/usr/bin/env python3
import base64
import json
import os
from http.server import HTTPServer, BaseHTTPRequestHandler

USERS = json.loads(os.environ.get("SUB_USERS_JSON", "{}"))
SERVER_IP = os.environ.get("SUB_SERVER_IP", "185.128.139.68")

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        user = self.path.strip("/")
        if user not in USERS:
            self.send_response(404)
            self.end_headers()
            return

        uuid = USERS[user]
        link = f"vless://{uuid}@{SERVER_IP}:443?encryption=none&security=none&type=ws&path=%2Fvless#{user}"
        content = base64.b64encode(link.encode()).decode()

        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.end_headers()
        self.wfile.write(content.encode())

HTTPServer(("127.0.0.1", 8089), Handler).serve_forever()
