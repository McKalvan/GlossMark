# @gloss implements .BearerAuthentication scope=file
"""Bearer-token authentication and role checks.

Every request is authenticated by `authenticate`, registered as a
before_request hook. Views opt out with `@public`; views that need a role
add `@requires_role`.
"""
from __future__ import annotations

import functools
from dataclasses import dataclass

import jwt
from flask import Flask, current_app, g, request
from werkzeug.exceptions import Forbidden, Unauthorized

_PUBLIC_ATTR = "_auth_public"


@dataclass(frozen=True)
class Principal:
    subject: str
    roles: frozenset[str]


def public(view):
    setattr(view, _PUBLIC_ATTR, True)
    return view


def authenticate() -> None:
    view = current_app.view_functions.get(request.endpoint or "")
    if view is None or getattr(view, _PUBLIC_ATTR, False):
        return
    header = request.headers.get("Authorization", "")
    scheme, _, token = header.partition(" ")
    if scheme.lower() != "bearer" or not token:
        raise Unauthorized("missing bearer token")
    try:
        claims = jwt.decode(
            token,
            key=current_app.config["JWT_PUBLIC_KEY"],
            algorithms=["RS256"],
            audience=current_app.config["JWT_AUDIENCE"],
            issuer=current_app.config["JWT_ISSUER"],
        )
    except jwt.PyJWTError as exc:
        raise Unauthorized("invalid token") from exc
    g.principal = Principal(subject=claims["sub"], roles=frozenset(claims.get("roles", [])))


def requires_role(role: str):
    def decorator(view):
        @functools.wraps(view)
        def wrapper(*args, **kwargs):
            principal: Principal | None = g.get("principal")
            if principal is None or role not in principal.roles:
                raise Forbidden(f"requires role {role}")
            return view(*args, **kwargs)

        return wrapper

    return decorator


def init_app(app: Flask) -> None:
    app.before_request(authenticate)
