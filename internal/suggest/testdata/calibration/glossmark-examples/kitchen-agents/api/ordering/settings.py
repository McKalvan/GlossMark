from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="ORDERING_", env_file=".env")

    database_url: str = "postgresql://ordering@localhost:5432/ordering"
    payment_provider_url: str = "https://api.tillpoint.example/v2"
    payment_provider_key: str = ""
    payment_webhook_secret: str = ""
    kds_push_url: str = "http://kds-gateway:8090/tickets"
    default_currency: str = "GBP"
    # Every station must see a ticket within this many seconds of the order being fired.
    ticket_push_timeout_s: float = 2.5


@lru_cache
def get_settings() -> Settings:
    return Settings()
