# @gloss implements .ExpensesApi scope=file
"""Application factory for the expense claims API."""
from flask import Flask

from app.handlers import claims, exports, health
from app.platform import auth, flags, log_setup, ratelimit


def create_app(config_dir: str = "config") -> Flask:
    log_setup.configure_logging(f"{config_dir}/logging.yaml")

    app = Flask(__name__)
    app.config["JWT_ISSUER"] = "https://id.tallyfern.example/"
    app.config["JWT_AUDIENCE"] = "expenses-api"

    flags.init_app(app, f"{config_dir}/flags.yaml")
    ratelimit.init_app(app, f"{config_dir}/ratelimit.yaml")
    auth.init_app(app)
    log_setup.init_app(app)

    app.register_blueprint(health.bp)
    app.register_blueprint(claims.bp, url_prefix="/claims")
    app.register_blueprint(exports.bp, url_prefix="/exports")
    return app
