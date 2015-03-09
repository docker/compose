from docker import Client
from docker import tls
import ssl
import os


def docker_client():
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    cert_path = os.environ.get('DOCKER_CERT_PATH', '')
    if cert_path == '':
        cert_path = os.path.join(os.environ.get('HOME', ''), '.docker')

    base_url = os.environ.get('DOCKER_HOST')
    tls_config = None

    if os.environ.get('DOCKER_TLS_VERIFY', '') != '':
        parts = base_url.split('://', 1)
        base_url = '%s://%s' % ('https', parts[1])

        client_cert = (os.path.join(cert_path, 'cert.pem'), os.path.join(cert_path, 'key.pem'))
        ca_cert = os.path.join(cert_path, 'ca.pem')

        tls_config = tls.TLSConfig(
            ssl_version=ssl.PROTOCOL_TLSv1,
            verify=True,
            assert_hostname=False,
            client_cert=client_cert,
            ca_cert=ca_cert,
        )

    timeout = int(os.environ.get('DOCKER_CLIENT_TIMEOUT', 60))
    return Client(base_url=base_url, tls=tls_config, version='1.17', timeout=timeout)
