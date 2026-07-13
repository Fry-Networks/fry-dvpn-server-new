use serde_json::Value;
use std::process::{Child, Command};
use thiserror::Error;

#[derive(Error, Debug)]
pub enum SupervisorError {
    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),
    #[error("Health check error: {0}")]
    HealthCheckError(String),
    #[error("Parse error: {0}")]
    ParseError(String),
    #[error("HTTP error: {0}")]
    HttpError(String),
}

pub trait HealthFetcher {
    fn get(&self, url: &str) -> Result<String, SupervisorError>;
}

pub struct DefaultHealthFetcher;

impl HealthFetcher for DefaultHealthFetcher {
    fn get(&self, url: &str) -> Result<String, SupervisorError> {
        ureq::get(url)
            .call()
            .map_err(|e| SupervisorError::HttpError(e.to_string()))
            .and_then(|resp| {
                resp.into_string()
                    .map_err(|e| SupervisorError::HttpError(e.to_string()))
            })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessStatus {
    Stopped,
    Starting,
    Running,
    Unhealthy,
}

pub trait Supervised {
    fn spawn_command(&self) -> Command;
    fn id(&self) -> &str;
    fn display_name(&self) -> &str;
    fn health_url(&self) -> String;
}

pub struct ProcessSupervisor<S: Supervised> {
    integration: S,
    child: Option<Child>,
}

impl<S: Supervised> ProcessSupervisor<S> {
    pub fn new(integration: S) -> Self {
        ProcessSupervisor {
            integration,
            child: None,
        }
    }

    pub fn start(&mut self) -> Result<(), SupervisorError> {
        if self.is_running()? {
            return Ok(());
        }
        let child = self.integration.spawn_command().spawn()?;
        self.child = Some(child);
        Ok(())
    }

    pub fn stop(&mut self) -> Result<(), SupervisorError> {
        if let Some(mut child) = self.child.take() {
            child.kill()?;
            let _ = child.wait();
        }
        Ok(())
    }

    pub fn is_running(&mut self) -> Result<bool, SupervisorError> {
        match &mut self.child {
            None => Ok(false),
            Some(child) => {
                match child.try_wait()? {
                    None => Ok(true),
                    Some(_) => {
                        self.child = None;
                        Ok(false)
                    }
                }
            }
        }
    }
}

pub struct HealthChecker<S: Supervised, F: HealthFetcher> {
    integration: S,
    fetcher: F,
}

impl<S: Supervised, F: HealthFetcher> HealthChecker<S, F> {
    pub fn new(integration: S, fetcher: F) -> Self {
        HealthChecker {
            integration,
            fetcher,
        }
    }

    pub fn check_health(&self) -> Result<ProcessStatus, SupervisorError> {
        let url = self.integration.health_url();
        let response = self.fetcher.get(&url)?;

        let json: Value = serde_json::from_str(&response)
            .map_err(|e| SupervisorError::ParseError(format!("Failed to parse JSON: {}", e)))?;

        let status = json
            .get("status")
            .and_then(|v| v.as_str())
            .unwrap_or("");

        let registered = json
            .get("registered")
            .and_then(|v| v.as_bool())
            .unwrap_or(false);

        if status == "healthy" && registered {
            Ok(ProcessStatus::Running)
        } else {
            Ok(ProcessStatus::Unhealthy)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct MockHealthFetcher {
        response: String,
    }

    impl HealthFetcher for MockHealthFetcher {
        fn get(&self, _url: &str) -> Result<String, SupervisorError> {
            Ok(self.response.clone())
        }
    }

    struct MockIntegration;

    impl Supervised for MockIntegration {
        fn spawn_command(&self) -> Command {
            #[cfg(target_os = "windows")]
            return Command::new("cmd");
            #[cfg(not(target_os = "windows"))]
            return Command::new("sh");
        }

        fn id(&self) -> &str {
            "mock"
        }

        fn display_name(&self) -> &str {
            "Mock Integration"
        }

        fn health_url(&self) -> String {
            "http://127.0.0.1:8088/health".to_string()
        }
    }

    #[test]
    fn test_health_check_running() {
        let mock_int = MockIntegration;
        let response = r#"{"status":"healthy","registered":true,"last_heartbeat_round":100}"#;
        let fetcher = MockHealthFetcher {
            response: response.to_string(),
        };
        let checker = HealthChecker::new(mock_int, fetcher);
        let status = checker.check_health().unwrap();
        assert_eq!(status, ProcessStatus::Running);
    }

    #[test]
    fn test_health_check_unhealthy_not_registered() {
        let mock_int = MockIntegration;
        let response = r#"{"status":"healthy","registered":false,"last_heartbeat_round":100}"#;
        let fetcher = MockHealthFetcher {
            response: response.to_string(),
        };
        let checker = HealthChecker::new(mock_int, fetcher);
        let status = checker.check_health().unwrap();
        assert_eq!(status, ProcessStatus::Unhealthy);
    }

    #[test]
    fn test_health_check_unhealthy_status() {
        let mock_int = MockIntegration;
        let response = r#"{"status":"unhealthy","registered":true,"last_heartbeat_round":0}"#;
        let fetcher = MockHealthFetcher {
            response: response.to_string(),
        };
        let checker = HealthChecker::new(mock_int, fetcher);
        let status = checker.check_health().unwrap();
        assert_eq!(status, ProcessStatus::Unhealthy);
    }

    #[test]
    fn test_health_check_invalid_json() {
        let mock_int = MockIntegration;
        let fetcher = MockHealthFetcher {
            response: "invalid json".to_string(),
        };
        let checker = HealthChecker::new(mock_int, fetcher);
        let result = checker.check_health();
        assert!(result.is_err());
    }
}
