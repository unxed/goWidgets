# Reference acquisition notes

- URL: `http://2g0.ru/fs/FSViewerSetup56.exe`
- Downloaded size: 6,389,255 bytes
- SHA-256: `1F6D5826D05046462CEEC503812A89E8347A68BC382678DB1D88BAEDC070C604`
- Local temporary file: `%TEMP%\FSViewerSetup56.exe` (not part of the repository)
- File signature: `MZ` (Windows PE executable)
- Installer family: NSIS 2 (LZMA solid archive).
- Extraction method: 7-Zip 26.03 x64; 183 files extracted to `%TEMP%\imgy-reference-5.6` (outside the repository).
- Help: primary English `CHM`, extracted to `%TEMP%\imgy-help-5.6`; table of contents is `Table of Contents.hhc` (1,384 bytes, ASCII-compatible HTML without a BOM).
- Help pages declare `iso-8859-1`; confirm per-page as needed when reading non-ASCII text.
- The license section is not a product feature. It contains restrictions against modifying or reverse-engineering the reference binary; only its handling implications were noted, and its text, the binary, and extracted files are not committed.
- No legal interpretation is made here; this project is based on the help and independently written code, not on modifying or redistributing the reference program.
- `Contact Us` contains publisher/support information and no user-facing feature requirements; its contact details are not copied into the project.
- `winget install 7zip.7zip --scope user` — no applicable installer (manifest provides MSI only).
- User approved installing 7-Zip. The official MSI was downloaded and hash-checked, then installed successfully.
- Wine is not installed/on PATH; no Wine installation is required for the currently authorized A1 extraction step.
