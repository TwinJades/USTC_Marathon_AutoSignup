param([ValidateSet('chrome','edge')][string]$Browser = 'chrome')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$portLines = Get-Content -LiteralPath (Join-Path $projectRoot "runtime\sessions\$Browser\DevToolsActivePort")
$targets = Invoke-RestMethod -Uri ('http://127.0.0.1:' + $portLines[0] + '/json/list')
$page = @($targets | Where-Object { $_.type -eq 'page' -and ([uri]$_.url).Host -in @('young.ustc.edu.cn','id.ustc.edu.cn','passport.ustc.edu.cn') })
if ($page.Count -ne 1) { throw '程序浏览器页面不唯一。' }
$socket = [System.Net.WebSockets.ClientWebSocket]::new()
$token = [System.Threading.CancellationTokenSource]::new(10000)
$socket.ConnectAsync([uri]$page[0].webSocketDebuggerUrl,$token.Token).GetAwaiter().GetResult() | Out-Null
$expression = @'
({host:location.hostname,path:location.pathname,ready:document.readyState,app:!!document.querySelector('#app'),vue:!!document.querySelector('#app')?.__vue__,bodyLength:document.body.innerText.length,inputs:[...document.querySelectorAll('input')].map(e=>({type:e.type,id:e.id,placeholder:e.placeholder})),scriptTags:[...document.querySelectorAll('script')].map(e=>({path:e.src?new URL(e.src).pathname:'',type:e.type,inlineLength:e.textContent.length})),scripts:performance.getEntriesByType('resource').filter(e=>e.initiatorType==='script').map(e=>({path:new URL(e.name).pathname,status:e.responseStatus,bytes:e.transferSize}))})
'@
$request = @{id=1;method='Runtime.evaluate';params=@{expression=$expression;returnByValue=$true}} | ConvertTo-Json -Depth 5 -Compress
$bytes = [System.Text.Encoding]::UTF8.GetBytes($request)
$socket.SendAsync([System.ArraySegment[byte]]::new($bytes),'Text',$true,$token.Token).GetAwaiter().GetResult() | Out-Null
$buffer = [byte[]]::new(65536)
$message = [System.IO.MemoryStream]::new()
do {
    $received = $socket.ReceiveAsync([System.ArraySegment[byte]]::new($buffer),$token.Token).GetAwaiter().GetResult()
    $message.Write($buffer,0,$received.Count)
} while (-not $received.EndOfMessage)
$reply = [System.Text.Encoding]::UTF8.GetString($message.ToArray()) | ConvertFrom-Json
$reply.result.result.value | ConvertTo-Json -Depth 8
$socket.Dispose()
$token.Dispose()
$message.Dispose()
