export interface PlatformInfo {
  os: string;
  arch: string;
}

export const detectPlatform = (): PlatformInfo => {
  let os: string;
  switch (process.platform) {
    case "darwin":
      os = "Darwin";
      break;
    case "linux":
      os = "Linux";
      break;
    case "win32":
      os = "Windows";
      break;
    default:
      throw new Error(`Unsupported operating system: ${process.platform}`);
  }

  let arch: string;
  switch (process.arch) {
    case "x64":
      arch = "x86_64";
      break;
    case "arm64":
      arch = "arm64";
      break;
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }

  return { os, arch };
};

export const getDownloadUrl = (version: string): string => {
  const platform = detectPlatform();
  const ext = process.platform === "win32" ? "zip" : "tar.gz";
  return `https://github.com/nektos/act/releases/download/v${version}/act_${platform.os}_${platform.arch}.${ext}`;
};
