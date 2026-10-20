FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /app
COPY pyproject.toml ./
COPY reminders ./reminders
RUN pip install --no-cache-dir .
USER 65532
CMD ["reminders-worker"]
