FROM python:3.12-slim AS base
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY pyproject.toml ./
RUN pip install --no-cache-dir .
COPY ordering ./ordering
USER 10001
EXPOSE 8080
CMD ["uvicorn", "ordering.main:app", "--host", "0.0.0.0", "--port", "8080"]
