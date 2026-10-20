FROM python:3.12-slim AS base
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
RUN pip install --no-cache-dir uv
WORKDIR /app
COPY services/assessment/pyproject.toml ./
RUN uv pip install --system -r pyproject.toml
COPY services/assessment/assessment ./assessment
COPY services/assessment/config ./config
COPY schemas /schemas
RUN useradd --system assessment
USER assessment
ENV SCHEMA_DIR=/schemas CONSUMER_CONFIG=/app/config/consumer-group.yaml
CMD ["python", "-m", "assessment.main"]
