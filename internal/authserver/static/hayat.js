// Hosted sign-in pages: a show/hide toggle on every password field. The pages
// work without it; this only adds the button.
(function () {
  var eye = '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.06 12.35a1 1 0 0 1 0-.7 10.75 10.75 0 0 1 19.88 0 1 1 0 0 1 0 .7 10.75 10.75 0 0 1-19.88 0"/><circle cx="12" cy="12" r="3"/></svg>';
  var eyeOff = '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.53 13.53 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><path d="M14.12 14.12a3 3 0 1 1-4.24-4.24"/><path d="m2 2 20 20"/></svg>';

  document.querySelectorAll('input[type=password]').forEach(function (input) {
    var wrap = document.createElement('span');
    wrap.className = 'pw';
    input.parentNode.insertBefore(wrap, input);
    wrap.appendChild(input);

    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'pw-toggle';
    var set = function (shown) {
      input.type = shown ? 'text' : 'password';
      btn.innerHTML = shown ? eyeOff : eye;
      btn.setAttribute('aria-pressed', String(shown));
      btn.setAttribute('aria-label', shown ? 'Hide password' : 'Show password');
    };
    set(false);
    btn.addEventListener('click', function () {
      set(input.type === 'password');
      input.focus();
    });
    // Never submit a password field as plain text left visible.
    if (input.form) input.form.addEventListener('submit', function () { set(false); });
    wrap.appendChild(btn);
  });
})();

// Passkeys: a button with data-passkey="login|mfa|reauth|register" runs a
// WebAuthn ceremony against /passkey/begin and /passkey/finish, then goes
// where the server says. The buttons stay hidden in browsers without
// passkeys, and every page still works without them.
(function () {
  var buttons = document.querySelectorAll('[data-passkey]');
  if (!buttons.length || !window.PublicKeyCredential || !navigator.credentials) return;

  function fromB64(s) {
    s = s.replace(/-/g, '+').replace(/_/g, '/');
    while (s.length % 4) s += '=';
    var bin = atob(s), out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out.buffer;
  }
  function toB64(buf) {
    var bytes = new Uint8Array(buf), bin = '';
    for (var i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }
  function ids(list) {
    (list || []).forEach(function (c) { c.id = fromB64(c.id); });
  }
  function encode(cred) {
    var r = cred.response, out = {
      id: cred.id, rawId: toB64(cred.rawId), type: cred.type,
      clientExtensionResults: cred.getClientExtensionResults ? cred.getClientExtensionResults() : {},
      response: { clientDataJSON: toB64(r.clientDataJSON) }
    };
    if (r.attestationObject) {
      out.response.attestationObject = toB64(r.attestationObject);
      if (r.getTransports) out.response.transports = r.getTransports();
    } else {
      out.response.authenticatorData = toB64(r.authenticatorData);
      out.response.signature = toB64(r.signature);
      if (r.userHandle) out.response.userHandle = toB64(r.userHandle);
    }
    return out;
  }
  function post(url, csrf, body) {
    return fetch(url, {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
      body: JSON.stringify(body)
    }).then(function (res) {
      return res.json().then(function (data) {
        if (!res.ok) throw new Error(data.error || 'Something went wrong.');
        return data;
      });
    });
  }

  buttons.forEach(function (btn) {
    var purpose = btn.getAttribute('data-passkey');
    var scope = btn.closest('.card') || document;
    var error = scope.querySelector('[data-passkey-error]');
    var nameInput = scope.querySelector('[data-passkey-name]');
    btn.hidden = false;
    scope.querySelectorAll('[data-passkey-only]').forEach(function (el) { el.hidden = false; });

    btn.addEventListener('click', function () {
      if (error) error.hidden = true;
      btn.disabled = true;
      var body = {
        purpose: purpose,
        authRequestID: btn.getAttribute('data-auth-request') || '',
        returnTo: btn.getAttribute('data-return-to') || ''
      };
      var csrf = btn.getAttribute('data-csrf');
      post('/passkey/begin', csrf, body).then(function (opts) {
        var pk = opts.publicKey;
        pk.challenge = fromB64(pk.challenge);
        if (purpose === 'register') {
          pk.user.id = fromB64(pk.user.id);
          ids(pk.excludeCredentials);
          return navigator.credentials.create({ publicKey: pk });
        }
        ids(pk.allowCredentials);
        return navigator.credentials.get({ publicKey: pk });
      }).then(function (cred) {
        body.credential = encode(cred);
        if (nameInput) body.name = nameInput.value;
        return post('/passkey/finish', csrf, body);
      }).then(function (data) {
        window.location.assign(data.redirect);
      }).catch(function (err) {
        btn.disabled = false;
        if (!error) return;
        // The person closing the browser's prompt is not an error worth a red box.
        if (err && err.name === 'NotAllowedError') {
          error.textContent = 'No passkey was used. Try again, or sign in another way.';
        } else if (err && err.name === 'InvalidStateError') {
          error.textContent = 'This device already has a passkey for your account.';
        } else {
          error.textContent = (err && err.message) || 'That passkey didn’t work.';
        }
        error.hidden = false;
      });
    });
  });
})();
