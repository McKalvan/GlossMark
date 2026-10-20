"""Message templates. Never include the visit reason: SMS is not a private channel."""
from jinja2 import Environment, StrictUndefined

_env = Environment(undefined=StrictUndefined, autoescape=False)

TEMPLATES = {
    "reminder_lead": _env.from_string(
        "Larkspur Clinics: reminder of your appointment on {{ starts_at.strftime('%a %d %b at %H:%M') }}. "
        "Reply C to cancel. Cancelling less than 24 hours ahead counts as a late cancellation."
    ),
    "reminder_same_day": _env.from_string(
        "Larkspur Clinics: see you today at {{ starts_at.strftime('%H:%M') }}. Please arrive 10 minutes early."
    ),
}


def render(template: str, **context: object) -> str:
    return TEMPLATES[template].render(**context)
