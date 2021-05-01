# https://github.com/docker/docker-py/blob/master/docker/utils/ports.py
# but add env variable to port validation pattern if --no-interpolate enabled
import re


def build_port_bindings(ports, interpolate=True):
    port_bindings = {}
    for port in ports:
        internal_port_range, external_range = split_port(port, interpolate)
        add_port(port_bindings, internal_port_range, external_range)
    return port_bindings


def _raise_invalid_port(port):  # from docker.utils.ports
    raise ValueError('Invalid port "%s", should be '
                     '[[remote_ip:]remote_port[-remote_port]:]'
                     'port[/protocol]. Note: ports ranges can not '
                     'be validated in --no-interpolate mode' % port)


def validate_port_with_vars(port, interpolate=True):
    port = str(port)
    PORT_SPEC = re.compile(  # from docker.utils.ports
        "^"  # Match full string
        "("  # External part
        r"(\[?(?P<host>(([a-fA-F\d.:]+)|(\$\{[\w:-]+\})|(\$\w+)))\]?:)?"  # Address
        r"(?P<ext>(([\d]*)|(\$\{[\w:-]+\})|(\$\w+)))(-(?P<ext_end>[\d]+))?:"  # External range
        ")?"
        r"(?P<int>(([\d]+)|(\$\{[\w:-]+\})|(\$\w+)))(-(?P<int_end>[\d]+))?"  # Internal range
        "(?P<proto>/(udp|tcp|sctp))?"  # Protocol
        "$"  # Match full string
    )
    if interpolate:
        PORT_SPEC = re.compile(
            "^"  # Match full string
            "("  # External part
            r"(\[?(?P<host>[a-fA-F\d.:]+)\]?:)?"  # Address
            r"(?P<ext>[\d]*)(-(?P<ext_end>[\d]+))?:"  # External range
            ")?"
            r"(?P<int>[\d]+)(-(?P<int_end>[\d]+))?"  # Internal range
            "(?P<proto>/(udp|tcp|sctp))?"  # Protocol
            "$"  # Match full string
        )

    match = PORT_SPEC.match(port)
    if match is None:
        _raise_invalid_port(port)
    return match


def port_range(start, end, proto, randomly_available_port=False):
    if not start:
        return start
    if not end:
        return [start + proto]
    if randomly_available_port:
        return ['{}-{}'.format(start, end) + proto]
    return [str(port) + proto for port in range(int(start), int(end) + 1)]


def split_port(port, interpolate=True):
    match = validate_port_with_vars(port, interpolate)
    parts = match.groupdict()
    host = parts['host']
    proto = parts['proto'] or ''
    internal = port_range(parts['int'], parts['int_end'], proto)
    external = port_range(
        parts['ext'], parts['ext_end'], '', len(internal) == 1)
    if host is None:
        if external is not None and len(internal) != len(external):
            raise ValueError('Port ranges don\'t match in length')
        return internal, external
    else:
        if not external:
            external = [None] * len(internal)
        elif len(internal) != len(external):
            raise ValueError('Port ranges don\'t match in length')
        return internal, [(host, ext_port) for ext_port in external]


def add_port_mapping(port_bindings, internal_port, external):
    if internal_port in port_bindings:
        port_bindings[internal_port].append(external)
    else:
        port_bindings[internal_port] = [external]


def add_port(port_bindings, internal_port_range, external_range):
    if external_range is None:
        for internal_port in internal_port_range:
            add_port_mapping(port_bindings, internal_port, None)
    else:
        ports = zip(internal_port_range, external_range)
        for internal_port, external_port in ports:
            add_port_mapping(port_bindings, internal_port, external_port)
