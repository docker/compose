from compose import timeparse


def test_milli():
    assert timeparse.timeparse('5ms') == 0.005


def test_milli_float():
    assert timeparse.timeparse('50.5ms') == 0.0505


def test_second_milli():
    assert timeparse.timeparse('200s5ms') == 200.005


def test_second_milli_micro():
    assert timeparse.timeparse('200s5ms10us') == 200.00501


def test_second():
    assert timeparse.timeparse('200s') == 200


def test_second_as_float():
    assert timeparse.timeparse('20.5s') == 20.5


def test_minute():
    assert timeparse.timeparse('32m') == 1920


def test_hour_minute():
    assert timeparse.timeparse('2h32m') == 9120


def test_minute_as_float():
    assert timeparse.timeparse('1.5m') == 90


def test_hour_minute_second():
    assert timeparse.timeparse('5h34m56s') == 20096


def test_invalid_with_space():
    assert timeparse.timeparse('5h 34m 56s') is None


def test_invalid_with_comma():
    assert timeparse.timeparse('5h,34m,56s') is None


def test_invalid_with_empty_string():
    assert timeparse.timeparse('') is None
