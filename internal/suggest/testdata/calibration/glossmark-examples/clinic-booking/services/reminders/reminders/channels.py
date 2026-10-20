"""Delivery channels: SMS through SNS, email through SES."""
from typing import Protocol

import boto3

from .models import Channel


class Sender(Protocol):
    def send(self, to: str, body: str) -> str: ...


class SmsSender:
    def __init__(self, sender_id: str):
        self._sns = boto3.client("sns")
        self._sender_id = sender_id

    def send(self, to: str, body: str) -> str:
        resp = self._sns.publish(
            PhoneNumber=to,
            Message=body,
            MessageAttributes={"AWS.SNS.SMS.SenderID": {"DataType": "String", "StringValue": self._sender_id}},
        )
        return resp["MessageId"]


class EmailSender:
    def __init__(self, source: str):
        self._ses = boto3.client("sesv2")
        self._source = source

    def send(self, to: str, body: str) -> str:
        resp = self._ses.send_email(
            FromEmailAddress=self._source,
            Destination={"ToAddresses": [to]},
            Content={"Simple": {"Subject": {"Data": "Your Larkspur appointment"}, "Body": {"Text": {"Data": body}}}},
        )
        return resp["MessageId"]


def sender_for(channel: Channel, sms_sender_id: str, email_from: str) -> Sender:
    return SmsSender(sms_sender_id) if channel is Channel.SMS else EmailSender(email_from)
