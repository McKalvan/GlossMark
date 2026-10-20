import pymysql
import pymysql.cursors

from utils.misc import load_cfg

_conn = None


def get_conn():
    global _conn
    if _conn is None or not _conn.open:
        c = load_cfg()
        _conn = pymysql.connect(
            host=c.get("db", "host"),
            user=c.get("db", "user"),
            password=c.get("db", "pass"),
            database=c.get("db", "name"),
            cursorclass=pymysql.cursors.DictCursor,
            autocommit=True,
        )
    return _conn


def get_stuff(sql, args=None):
    with get_conn().cursor() as cur:
        cur.execute(sql, args)
        return cur.fetchall()


def save_stuff(sql, args=None):
    with get_conn().cursor() as cur:
        return cur.execute(sql, args)
