# ABOUTME: Hands one data-only file URL to the Windows default association.
# ABOUTME: Does not read profiles, run document strings, or wait for the browser.
try {
    $previewUrl = [Console]::In.ReadToEnd()
    Start-Process -FilePath $previewUrl -ErrorAction Stop | Out-Null
    exit 0
} catch {
    exit 1
}
