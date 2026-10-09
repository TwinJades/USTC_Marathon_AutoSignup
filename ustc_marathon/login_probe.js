() => {
  const visible = e => !!e.getClientRects().length && getComputedStyle(e).visibility !== 'hidden';
  const inputs = Array.from(document.querySelectorAll('input')).filter(visible);
  const metadata = e => [e.id, e.name, e.placeholder, e.autocomplete].filter(Boolean).join(' ');
  const verification = inputs.find(e => ['text','tel','number'].includes(e.type)
    && /captcha|otp|sms|verify|verification|验证码|短信|动态码/i.test(metadata(e)));
  const frame = Array.from(document.querySelectorAll('iframe')).some(e => visible(e)
    && /captcha|recaptcha|hcaptcha/i.test(e.src));
  if (verification || frame) return {kind: 'verification'};
  const passwords = inputs.filter(e => e.type === 'password');
  const accounts = inputs.filter(e => e.type !== 'password'
    && /username|account|login|学号|账号|工号|gid/i.test(metadata(e)));
  if (passwords.length === 1 && accounts.length === 1) {
    const all = Array.from(document.querySelectorAll('input'));
    return {kind: 'login', accountIndex: all.indexOf(accounts[0]), passwordIndex: all.indexOf(passwords[0])};
  }
  if (!passwords.length && /短信验证|新设备|二次认证|动态码|安全验证/.test(document.body?.innerText || '')) {
    return {kind: 'verification'};
  }
  return {kind: 'waiting'};
}
