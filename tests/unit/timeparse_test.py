from decimal import Decimal

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


def test_float_ending_in_dot():
    assert timeparse.timeparse('1.m') == 60


def test_float_starting_in_dot():
    assert timeparse.timeparse('.5m') == 30


def test_dot_alone_isnt_number():
    assert timeparse.timeparse('.m') is None


def test_multiple_dots_isnt_number():
    assert timeparse.timeparse('.0.m') is None
    assert timeparse.timeparse('0.0.m') is None
    assert timeparse.timeparse('00..m') is None
    assert timeparse.timeparse('..00m') is None


def test_hour_minute_second():
    assert timeparse.timeparse('5h34m56s') == 20096


def test_invalid_with_space():
    assert timeparse.timeparse('5h 34m 56s') is None


def test_invalid_with_comma():
    assert timeparse.timeparse('5h,34m,56s') is None


def test_invalid_with_empty_string():
    assert timeparse.timeparse('') is None


def test_precise_timing():
    assert timeparse.timeparse('0.9999999999999999s0.0000001ns') == 1.0
    assert timeparse.timeparse('0.9999999999999999s0.0000000ns') == 0.9999999999999999


def test_exact_timing():
    h = '10000'
    m = '0.00000001'
    s = '0.999999999'
    ms = '0.00000001'
    us = '0.999999999'
    ns = '0.00000001'

    below = Decimal(ns)
    below = below / 1000 + Decimal(us)
    below = below / 1000 + Decimal(ms)
    below /= 1000

    above = (Decimal(h) * 60 + Decimal(m)) * 60

    seconds = above + Decimal(s) + below

    timestamp = '{h}h{m}m{s}s{ms}ms{us}us{ns}ns'.format(h=h, m=m, s=s, ms=ms, us=us, ns=ns)

    assert timeparse.timeparse(timestamp) == float(seconds)
    assert timeparse.timeparse(timestamp, exact=True) == seconds


def test_helper_functions():
    n = Decimal('1234.567891234')

    units = ['hours', 'mins', 'secs', 'milli', 'micro', 'nano']

    for unit in units:
        assert n == timeparse.time_to_seconds(timeparse.time_from_seconds(n, unit=unit), unit=unit)
        assert n == timeparse.time_from_seconds(timeparse.time_to_seconds(n, unit=unit), unit=unit)
