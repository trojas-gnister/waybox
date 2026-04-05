pub fn map_package_name(name: &str) -> &str {
    match name {
        "python3" | "python" => "python3Full",
        "node" | "nodejs" => "nodejs",
        "vim" => "vim-full",
        "java" => "jdk",
        "go" => "go",
        _ => name,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_known_mappings() {
        assert_eq!(map_package_name("python3"), "python3Full");
        assert_eq!(map_package_name("python"), "python3Full");
        assert_eq!(map_package_name("vim"), "vim-full");
        assert_eq!(map_package_name("java"), "jdk");
    }

    #[test]
    fn test_passthrough() {
        assert_eq!(map_package_name("firefox"), "firefox");
        assert_eq!(map_package_name("git"), "git");
        assert_eq!(map_package_name("htop"), "htop");
    }
}
