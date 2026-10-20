import datetime


def to_int(v, default=0):
    try:
        return int(str(v).strip())
    except (TypeError, ValueError):
        return default


def safe_div(a, b):
    return a / b if b else 0.0


def chunk(seq, n):
    for i in range(0, len(seq), n):
        yield seq[i : i + n]


def now_str():
    return datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
