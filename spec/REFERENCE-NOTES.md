# Reference acquisition notes

- URL: `http://2g0.ru/fs/FSViewerSetup56.exe`
- Downloaded size: 6,389,255 bytes
- SHA-256: `1F6D5826D05046462CEEC503812A89E8347A68BC382678DB1D88BAEDC070C604`
- Local temporary file: `%TEMP%\FSViewerSetup56.exe` (not part of the repository)
- File signature: `MZ` (Windows PE executable)
- Installer family, extraction method, help format/path, and encoding: pending; extraction tools are not available on this Windows host.
- First prescribed attempt: `7z.exe l` — command not found.
- Second prescribed attempt: `innoextract.exe -l` — command not found.
- `winget install 7zip.7zip --scope user` — no applicable installer (manifest provides MSI only).
- User approved installing 7-Zip. The official MSI was downloaded and hash-checked; the elevated installation process is currently pending/completing. Confirm installation before extraction.
- Wine is not installed/on PATH; no Wine installation is required for the currently authorized A1 extraction step.
