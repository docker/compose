from docker import Client
from docker import tls as docker_tls
import ssl
import os
import warnings
from . import errors


def docker_client(tls=None, tls_ca_cert=None, tls_cert=None, tls_key=None, tls_verify=None):
    """
    Returns a docker-py client configured using environment variables
    according to the same logic as the official Docker client.
    """
    cert_path = os.environ.get('DOCKER_CERT_PATH', '')
    if cert_path == '':
        cert_path = os.path.join(os.environ.get('HOME', ''), '.docker')

    base_url = os.environ.get('DOCKER_HOST')
    tls_config = None

    tls = tls in ("true", "t", "1")
    tls_verify = os.environ.get('DOCKER_TLS_VERIFY', '') != '' if tls_verify is None else tls_verify in ("true", "t", "1")
    if tls or tls_verify:
        parts = base_url.split('://', 1)
        base_url = '%s://%s' % ('https', parts[1])

        # Prefer cli argument over environment variable
        tls_ca_cert = os.path.expanduser(os.path.join(cert_path, 'ca.pem') if tls_ca_cert is None else tls_ca_cert)
        tls_cert = os.path.expanduser(os.path.join(cert_path, 'cert.pem') if tls_cert is None else tls_cert)
        tls_key = os.path.expanduser(os.path.join(cert_path, 'key.pem') if tls_key is None else tls_key)

        if not tls_verify:
            warnings.warn((
                'Unverified HTTPS request is being made. '
                'Adding certificate verification is strongly advised. See: '
                'https://urllib3.readthedocs.org/en/latest/security.html'),
                errors.InsecureRequestWarning)
            tls_ca_cert = None

        tls_config = docker_tls.TLSConfig(
            ssl_version=ssl.PROTOCOL_TLSv1,
            verify=tls_verify,
            assert_hostname=False,
            client_cert=(tls_cert, tls_key),
            ca_cert=tls_ca_cert,
        )

    timeout = int(os.environ.get('DOCKER_CLIENT_TIMEOUT', 60))
    return Client(base_url=base_url, tls=tls_config, version='1.18', timeout=timeout)
