"""Runtime settings, read from the environment (see deploy/k8s/reminders/configmap.yaml)."""
from pydantic_settings import BaseSettings, SettingsConfigDict

# How long before an appointment the reminder goes out. Clinic operations own this number.
# @gloss publishes-value .LeadTimeSetting scope=line conf=0.9
REMINDER_LEAD_HOURS = 48

# A second, shorter nudge on the day itself.
SAME_DAY_NUDGE_HOURS = 3


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="REMINDERS_")

    database_url: str
    queue_url: str
    sms_sender_id: str = "LARKSPUR"
    email_from: str = "appointments@larkspur.example"
    batch_size: int = 25
    dry_run: bool = False
