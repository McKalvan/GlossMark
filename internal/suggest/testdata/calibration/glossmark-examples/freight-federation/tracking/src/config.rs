use std::env;
use std::net::SocketAddr;

#[derive(Debug, Clone)]
pub struct Config {
    pub database_url: String,
    pub database_max_connections: u32,
    pub kafka_brokers: String,
    pub kafka_group_id: String,
    pub http_addr: SocketAddr,
}

#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    #[error("missing environment variable {0}")]
    Missing(&'static str),
    #[error("invalid value for {name}: {value:?}")]
    Invalid { name: &'static str, value: String },
}

impl Config {
    pub fn from_env() -> Result<Self, ConfigError> {
        Ok(Self {
            database_url: required("DATABASE_URL")?,
            database_max_connections: parsed("DATABASE_MAX_CONNECTIONS", 10)?,
            kafka_brokers: required("KAFKA_BROKERS")?,
            kafka_group_id: env::var("KAFKA_GROUP_ID").unwrap_or_else(|_| "tracking".to_owned()),
            http_addr: parsed("HTTP_ADDR", SocketAddr::from(([0, 0, 0, 0], 8080)))?,
        })
    }
}

fn required(name: &'static str) -> Result<String, ConfigError> {
    env::var(name).map_err(|_| ConfigError::Missing(name))
}

fn parsed<T: std::str::FromStr>(name: &'static str, default: T) -> Result<T, ConfigError> {
    match env::var(name) {
        Ok(value) => value
            .parse()
            .map_err(|_| ConfigError::Invalid { name, value }),
        Err(_) => Ok(default),
    }
}
