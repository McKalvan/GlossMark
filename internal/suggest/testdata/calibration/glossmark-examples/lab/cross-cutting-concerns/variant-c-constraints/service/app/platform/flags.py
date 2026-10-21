# @gloss implements .FeatureFlags scope=file
"""Feature flags read from flags.yaml; a disabled flag hides its route."""
from __future__ import annotations

import functools

import yaml
from flask import Flask, current_app
from werkzeug.exceptions import NotFound


def is_enabled(name: str) -> bool:
    return bool(current_app.extensions["flags"].get(name, {}).get("enabled", False))


def feature(name: str):
    def decorator(view):
        @functools.wraps(view)
        def wrapper(*args, **kwargs):
            if not is_enabled(name):
                raise NotFound()
            return view(*args, **kwargs)

        return wrapper

    return decorator


def init_app(app: Flask, path: str) -> None:
    with open(path, encoding="utf-8") as fh:
        app.extensions["flags"] = yaml.safe_load(fh)["flags"]
