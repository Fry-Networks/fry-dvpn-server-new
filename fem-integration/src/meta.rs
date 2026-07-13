use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IntegrationMeta {
    pub id: String,
    pub name: String,
    pub description: String,
    pub category: String,
    pub icon: String,
}

impl IntegrationMeta {
    pub fn new() -> Self {
        IntegrationMeta {
            id: "fryvpn".to_string(),
            name: "Fry dVPN".to_string(),
            description: "Fry decentralized VPN node that provides bandwidth to the Fry network".to_string(),
            category: "Bandwidth".to_string(),
            icon: "fryvpn-icon".to_string(),
        }
    }
}

impl Default for IntegrationMeta {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_integration_meta_creation() {
        let meta = IntegrationMeta::new();
        assert_eq!(meta.id, "fryvpn");
        assert_eq!(meta.name, "Fry dVPN");
        assert_eq!(meta.category, "Bandwidth");
    }

    #[test]
    fn test_integration_meta_serialization() {
        let meta = IntegrationMeta::new();
        let json = serde_json::to_string(&meta).expect("Failed to serialize");
        assert!(json.contains("\"id\":\"fryvpn\""));
        assert!(json.contains("\"name\":\"Fry dVPN\""));
    }

    #[test]
    fn test_integration_meta_deserialization() {
        let json = r#"{"id":"fryvpn","name":"Fry dVPN","description":"Fry decentralized VPN node that provides bandwidth to the Fry network","category":"Bandwidth","icon":"fryvpn-icon"}"#;
        let meta: IntegrationMeta = serde_json::from_str(json).expect("Failed to deserialize");
        assert_eq!(meta.id, "fryvpn");
        assert_eq!(meta.name, "Fry dVPN");
    }

    #[test]
    fn test_integration_meta_default() {
        let meta = IntegrationMeta::default();
        assert_eq!(meta.id, "fryvpn");
    }
}
