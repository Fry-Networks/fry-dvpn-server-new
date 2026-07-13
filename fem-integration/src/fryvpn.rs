use crate::supervisor::{HealthChecker, DefaultHealthFetcher, ProcessStatus, ProcessSupervisor, Supervised, SupervisorError};
use std::process::Command;

pub struct FryVpnIntegration {
    binary_path: String,
}

impl FryVpnIntegration {
    pub fn new() -> Self {
        let binary_path = std::env::var("FRYNODE_BIN")
            .unwrap_or_else(|_| {
                #[cfg(target_os = "windows")]
                return "frynode.exe".to_string();
                #[cfg(not(target_os = "windows"))]
                return "frynode".to_string();
            });

        FryVpnIntegration { binary_path }
    }

    pub fn new_with_binary(binary_path: String) -> Self {
        FryVpnIntegration { binary_path }
    }

    pub fn start(&self) -> Result<ProcessSupervisor<Self>, SupervisorError> {
        let mut supervisor = ProcessSupervisor::new(FryVpnIntegration {
            binary_path: self.binary_path.clone(),
        });
        supervisor.start()?;
        Ok(supervisor)
    }

    pub fn check_health(&self) -> Result<ProcessStatus, SupervisorError> {
        let checker = HealthChecker::new(
            FryVpnIntegration {
                binary_path: self.binary_path.clone(),
            },
            DefaultHealthFetcher,
        );
        checker.check_health()
    }
}

impl Default for FryVpnIntegration {
    fn default() -> Self {
        Self::new()
    }
}

impl Supervised for FryVpnIntegration {
    fn spawn_command(&self) -> Command {
        let mut cmd = Command::new(&self.binary_path);

        if let Ok(registry_app_id) = std::env::var("REGISTRY_APP_ID") {
            cmd.env("REGISTRY_APP_ID", registry_app_id);
        }

        if let Ok(algod_server) = std::env::var("ALGOD_SERVER") {
            cmd.env("ALGOD_SERVER", algod_server);
        }

        if let Ok(region) = std::env::var("REGION") {
            cmd.env("REGION", region);
        }

        if let Ok(price_per_gb) = std::env::var("PRICE_PER_GB") {
            cmd.env("PRICE_PER_GB", price_per_gb);
        }

        if let Ok(wg_port) = std::env::var("WG_PORT") {
            cmd.env("WG_PORT", wg_port);
        }

        if let Ok(api_port) = std::env::var("API_PORT") {
            cmd.env("API_PORT", api_port);
        }

        cmd
    }

    fn id(&self) -> &str {
        "fryvpn"
    }

    fn display_name(&self) -> &str {
        "Fry dVPN"
    }

    fn health_url(&self) -> String {
        "http://127.0.0.1:8088/health".to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fryvpn_id() {
        let integration = FryVpnIntegration::new();
        assert_eq!(integration.id(), "fryvpn");
    }

    #[test]
    fn test_fryvpn_display_name() {
        let integration = FryVpnIntegration::new();
        assert_eq!(integration.display_name(), "Fry dVPN");
    }

    #[test]
    fn test_fryvpn_health_url() {
        let integration = FryVpnIntegration::new();
        assert_eq!(
            integration.health_url(),
            "http://127.0.0.1:8088/health"
        );
    }

    #[test]
    fn test_fryvpn_default() {
        let integration = FryVpnIntegration::default();
        assert_eq!(integration.id(), "fryvpn");
    }

    #[test]
    fn test_fryvpn_spawn_command_with_env_vars() {
        std::env::set_var("REGISTRY_APP_ID", "123456");
        std::env::set_var("REGION", "us-east-1");

        let integration = FryVpnIntegration::new_with_binary("test_binary".to_string());
        let _cmd = integration.spawn_command();

        std::env::remove_var("REGISTRY_APP_ID");
        std::env::remove_var("REGION");
    }
}
