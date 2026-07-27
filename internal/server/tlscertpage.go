package server

import "html/template"

// tlsCertPageTmpl renders the /api/v1/tls-cert page: the certificate's
// SHA-256 fingerprint and full PEM text, each with a "copy" button, and
// nothing else — deliberately no explanatory text, per explicit user
// request. Rendered via html/template (rather than a plain fmt.Sprintf) for
// contextually-correct escaping, matching cmd/oauthsetup's picker.html
// precedent — the content here is always server-generated, not user
// input, so the risk is minimal, but there's no reason not to do it
// safely by default.
var tlsCertPageTmpl = template.Must(template.New("tlscert").Parse(tlsCertPageHTML))

const tlsCertPageHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Certificado TLS</title>
<style>
  body { font-family: -apple-system, system-ui, sans-serif; max-width: 640px; margin: 2rem auto; padding: 0 1rem; color: #222; }
  h1 { font-size: 1.3rem; }
  h2 { font-size: 1.05rem; margin-top: 2rem; }
  pre { background: #f4f4f4; border: 1px solid #ddd; border-radius: 6px; padding: 0.75rem; overflow-wrap: break-word; white-space: pre-wrap; font-size: 0.85rem; }
  button { font-size: 1rem; padding: 0.5rem 1rem; border-radius: 6px; border: 1px solid #888; background: #eee; }
  button:active { background: #ddd; }
</style>
</head>
<body>
<h1>Certificado TLS de my-assistant</h1>

<h2>Fingerprint (SHA-256)</h2>
<pre id="fp">{{.Fingerprint}}</pre>
<button type="button" onclick="copyFrom('fp', this)">Copiar fingerprint</button>

<h2>Certificado completo (PEM)</h2>
<pre id="pem">{{.CertPEM}}</pre>
<button type="button" onclick="copyFrom('pem', this)">Copiar certificado (PEM)</button>

<script>
function copyFrom(id, btn) {
  var text = document.getElementById(id).innerText;
  var original = btn.textContent;
  function done() {
    btn.textContent = '¡Copiado!';
    setTimeout(function() { btn.textContent = original; }, 1500);
  }
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done, function() {
      alert('No se pudo copiar automáticamente. Selecciona el texto manualmente.');
    });
  } else {
    alert('El portapapeles no está disponible en este navegador. Selecciona el texto manualmente.');
  }
}
</script>
</body>
</html>
`
