import time

import jwt
import pytest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa

from app.main import create_app


@pytest.fixture(scope="session")
def keypair():
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    public_pem = key.public_key().public_bytes(
        serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo
    )
    return key, public_pem


@pytest.fixture()
def app(keypair, tmp_path):
    app = create_app("config")
    app.config["JWT_PUBLIC_KEY"] = keypair[1]
    return app


@pytest.fixture()
def token(keypair, app):
    def make(sub="emp-1", roles=()):
        return jwt.encode(
            {
                "sub": sub,
                "roles": list(roles),
                "aud": app.config["JWT_AUDIENCE"],
                "iss": app.config["JWT_ISSUER"],
                "exp": int(time.time()) + 300,
            },
            keypair[0],
            algorithm="RS256",
        )

    return make
