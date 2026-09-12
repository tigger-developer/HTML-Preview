# ABOUTME: Reads one loopback health URL as data and returns its bounded JSON response.
# ABOUTME: Disables proxies and redirects; the parent enforces the overall five-second deadline.
$previewResponse = $null
$previewReader = $null
try {
    $previewUrl = [Console]::In.ReadToEnd()
    $previewRequest = [System.Net.HttpWebRequest]::Create($previewUrl)
    $previewRequest.Proxy = $null
    $previewRequest.AllowAutoRedirect = $false
    $previewRequest.Timeout = 5000
    $previewRequest.ReadWriteTimeout = 5000
    $previewResponse = $previewRequest.GetResponse()
    if ([int]$previewResponse.StatusCode -ne 200 -or $previewResponse.ContentType -ne 'application/json') {
        throw 'Health response refused'
    }
    $previewReader = [System.IO.StreamReader]::new($previewResponse.GetResponseStream())
    $previewBuffer = [char[]]::new(1025)
    $previewCount = 0
    while ($previewCount -lt $previewBuffer.Length) {
        $previewRead = $previewReader.Read($previewBuffer, $previewCount, $previewBuffer.Length - $previewCount)
        if ($previewRead -eq 0) { break }
        $previewCount += $previewRead
    }
    if ($previewCount -eq 0 -or $previewCount -gt 1024) { throw 'Health response exceeds bounds' }
    [Console]::Out.Write(-join $previewBuffer[0..($previewCount - 1)])
} catch {
    exit 1
} finally {
    if ($null -ne $previewReader) { $previewReader.Dispose() }
    if ($null -ne $previewResponse) { $previewResponse.Close() }
}
