import configparser
import logging
import smtplib
from email.message import EmailMessage

log = logging.getLogger("opstool")


def load_cfg(path="/etc/opstool.ini"):
    c = configparser.ConfigParser()
    c.read(path)
    return c


def send_mail(to, subject, body):
    m = EmailMessage()
    m["From"] = "opstool@acme.local"
    m["To"] = to
    m["Subject"] = subject
    m.set_content(body)
    with smtplib.SMTP("mail.acme.local") as s:
        s.send_message(m)


def log_it(*args):
    log.info(" ".join(str(a) for a in args))
