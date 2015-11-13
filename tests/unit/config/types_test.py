from compose.config.types import parse_extra_hosts


def test_parse_extra_hosts_list():
    expected = {'www.example.com': '192.168.0.17'}
    assert parse_extra_hosts(["www.example.com:192.168.0.17"]) == expected

    expected = {'www.example.com': '192.168.0.17'}
    assert parse_extra_hosts(["www.example.com: 192.168.0.17"]) == expected

    assert parse_extra_hosts([
        "www.example.com: 192.168.0.17",
        "static.example.com:192.168.0.19",
        "api.example.com: 192.168.0.18"
    ]) == {
        'www.example.com': '192.168.0.17',
        'static.example.com': '192.168.0.19',
        'api.example.com': '192.168.0.18'
    }


def test_parse_extra_hosts_dict():
    assert parse_extra_hosts({
        'www.example.com': '192.168.0.17',
        'api.example.com': '192.168.0.18'
    }) == {
        'www.example.com': '192.168.0.17',
        'api.example.com': '192.168.0.18'
    }
