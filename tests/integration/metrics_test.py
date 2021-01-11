import logging
import os
import socket
from http.server import BaseHTTPRequestHandler
from http.server import HTTPServer
from threading import Thread

import requests
from docker.transport import UnixHTTPAdapter

from tests.acceptance.cli_test import dispatch
from tests.integration.testcases import DockerClientTestCase


TEST_SOCKET_FILE = '/tmp/test-metrics-docker-cli.sock'


class MetricsTest(DockerClientTestCase):
    test_session = requests.sessions.Session()
    test_env = None
    base_dir = 'tests/fixtures/v3-full'

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        MetricsTest.test_session.mount("http+unix://", UnixHTTPAdapter(TEST_SOCKET_FILE))
        MetricsTest.test_env = os.environ.copy()
        MetricsTest.test_env['METRICS_SOCKET_FILE'] = TEST_SOCKET_FILE
        MetricsServer().start()

    @classmethod
    def test_metrics_help(cls):
        # root `docker-compose` command is considered as a `--help`
        dispatch(cls.base_dir, [], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose --help", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'
        dispatch(cls.base_dir, ['help', 'run'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose help", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'
        dispatch(cls.base_dir, ['--help'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose --help", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'
        dispatch(cls.base_dir, ['run', '--help'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose --help run", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'
        dispatch(cls.base_dir, ['up', '--help', 'extra_args'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose --help up", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'

    @classmethod
    def test_metrics_simple_commands(cls):
        dispatch(cls.base_dir, ['ps'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose ps", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'
        dispatch(cls.base_dir, ['version'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose version", "context": "moby", ' \
               b'"source": "docker-compose", "status": "success"}'
        dispatch(cls.base_dir, ['version', '--yyy'], env=MetricsTest.test_env)
        assert cls.get_content() == \
               b'{"command": "compose version", "context": "moby", ' \
               b'"source": "docker-compose", "status": "failure"}'

    @staticmethod
    def get_content():
        resp = MetricsTest.test_session.get("http+unix://localhost")
        print(resp.content)
        return resp.content


def start_server(uri=TEST_SOCKET_FILE):
    try:
        os.remove(uri)
    except OSError:
        pass
    httpd = HTTPServer(uri, MetricsHTTPRequestHandler, False)
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.bind(TEST_SOCKET_FILE)
    sock.listen(0)
    httpd.socket = sock
    print('Serving on ', uri)
    httpd.serve_forever()
    sock.shutdown(socket.SHUT_RDWR)
    sock.close()
    os.remove(uri)


class MetricsServer:
    @classmethod
    def start(cls):
        t = Thread(target=start_server, daemon=True)
        t.start()


class MetricsHTTPRequestHandler(BaseHTTPRequestHandler):
    usages = []

    def do_GET(self):
        self.client_address = ('',)  # avoid exception in BaseHTTPServer.py log_message()
        self.send_response(200)
        self.end_headers()
        for u in MetricsHTTPRequestHandler.usages:
            self.wfile.write(u)
        MetricsHTTPRequestHandler.usages = []

    def do_POST(self):
        self.client_address = ('',)  # avoid exception in BaseHTTPServer.py log_message()
        content_length = int(self.headers['Content-Length'])
        body = self.rfile.read(content_length)
        print(body)
        MetricsHTTPRequestHandler.usages.append(body)
        self.send_response(200)
        self.end_headers()


if __name__ == '__main__':
    logging.getLogger("urllib3").propagate = False
    logging.getLogger("requests").propagate = False
    start_server()
