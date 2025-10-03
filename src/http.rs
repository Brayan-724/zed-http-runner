use std::fs;
use std::path::PathBuf;

use zed_extension_api as zed;

struct HttpExtension {
    binary_path: Option<String>,
}

impl HttpExtension {
    fn load_response_viewer(
        &mut self,
        language_server_id: &zed::LanguageServerId,
        worktree: &zed::Worktree,
    ) -> zed::Result<String> {
        if let Some(dev_path) = worktree
            .shell_env()
            .into_iter()
            .find_map(|(k, v)| (k == "ZED_HTTP_RESPONSE_VIEWER").then_some(v))
        {
            let dev_path = PathBuf::from(worktree.root_path())
                .join(dev_path)
                .to_string_lossy()
                .to_string();

            return Ok(dev_path);
        }

        if let Some(binary_path) = self.binary_path.clone() {
            return Ok(binary_path);
        }

        zed::set_language_server_installation_status(
            language_server_id,
            &zed::LanguageServerInstallationStatus::CheckingForUpdate,
        );
        let release = zed::latest_github_release(
            "Brayan-724/zed-http-runner",
            zed::GithubReleaseOptions {
                require_assets: true,
                pre_release: false,
            },
        )?;

        // let (platform, arch) = zed::current_platform();
        let asset_name = format!(
            "zed-http-response-viewer",
            // arch = match arch {
            //     zed::Architecture::Aarch64 => "aarch64",
            //     zed::Architecture::X86 => "x86",
            //     zed::Architecture::X8664 => "x86_64",
            // },
            // os = match platform {
            //     zed::Os::Mac => "apple",
            //     zed::Os::Linux => "linux",
            //     zed::Os::Windows => "windows",
            // },
        );

        let asset = release
            .assets
            .iter()
            .find(|asset| asset.name == asset_name)
            .ok_or_else(|| format!("no asset found matching {asset_name:?}"))?;

        let binary_path = format!("zed-http-response-viewer-{}", release.version);

        if let Err(_) | Ok(false) = fs::exists(&binary_path) {
            zed::set_language_server_installation_status(
                language_server_id,
                &zed::LanguageServerInstallationStatus::Downloading,
            );

            zed::download_file(
                &asset.download_url,
                &binary_path,
                zed::DownloadedFileType::Uncompressed,
            )
            .map_err(|e| format!("failed to download file: {e}"))?;
        }

        self.binary_path = Some(binary_path.clone());

        Ok(binary_path)
    }
}

impl zed::Extension for HttpExtension {
    fn new() -> Self {
        Self { binary_path: None }
    }

    fn language_server_command(
        &mut self,
        language_server_id: &zed::LanguageServerId,
        worktree: &zed::Worktree,
    ) -> zed::Result<zed::Command> {
        Ok(zed::Command {
            command: self.load_response_viewer(language_server_id, worktree)?,
            args: vec!["--setup".to_owned()],
            env: vec![],
        })
    }
}

zed::register_extension!(HttpExtension);
