let ws, recorder
const button = document.getElementById('hitme')
const logbinmsg = document.getElementById('log_bin_msg')

function unhandledError (err) {
  console.error('UNHANDLED:', err)
}

function log (text, classes, html) {
  let row = document.createElement('div')
  row.classList.add('row')
  classes && classes.forEach(c => row.classList.add(c))
  row[html ? 'innerHTML' : 'innerText'] = text
  document.body.prepend(row)
}
function comment (text) {
  log('# ' + text, ['comment'])
}

//https://stackoverflow.com/questions/4810841/pretty-print-json-using-javascript
function json2html (json) {
  json = json
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  return json.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
    function (match) {
      var cls = 'number'
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          cls = 'key'
        } else {
          cls = 'string'
        }
      } else if (/true|false/.test(match)) {
        cls = 'boolean'
      } else if (/null/.test(match)) {
        cls = 'null'
      }
      return '<span class="' + cls + '">' + match + '</span>'
    }
  )
}

function enableRecorder (on) {
  let prev = recorder.state
  if (on && recorder.state == 'inactive') recorder.start(200)
  if (!on && recorder.state == 'recording') recorder.stop()
  let cur = recorder.state
  if (cur != prev) comment('recorder state changed: ' + prev + ' -> ' + cur)
  button.removeAttribute('disabled')
}

function manageWebsocket () {
  if (ws) ws.close()
  ws = new WebSocket('ws://' + document.location.host + '/v1/ws')
  ws.onopen = e => {
    button.setAttribute('disabled', true)
    comment('websocket open')
  }
  ws.onclose = e => {
    comment('websocket closed: ' + e.reason)
    enableRecorder(false)
    setTimeout(manageWebsocket, 5000)
  }
  ws.onerror = e => {
    console.error('websocket error:', e)
    enableRecorder(false)
    ws.close()
  }
  ws.onmessage = function (evt) {
    let p = JSON.parse(evt.data)
    log('&lt;&nbsp;TXT;&nbsp;' + json2html(JSON.stringify(p)), ['rx'], true)
    if (!p) return

    if (p.event == 'status_changed' && p.result == true) {
      button.removeAttribute('disabled')
    }
  }
}

function sendBytes (payload) {
  if (logbinmsg.checked) {
    log('> BIN; ' + payload.type + ' | ' + payload.size + ' bytes', ['tx'])
  }
  return ws.send(payload)
}

button.onclick = e => enableRecorder(recorder.state == 'inactive')

function main (audioStream) {
  recorder = new MediaRecorder(audioStream)
  recorder.ondataavailable = e => {
    sendBytes(e.data)
    if (recorder.state == 'inactive') {
      comment('recorder drained')
    }
  }
  manageWebsocket()
}

if (!navigator.mediaDevices) {
  comment('no browser media device available :-(')
} else {
  navigator.mediaDevices
    .getUserMedia({ audio: true })
    .then(main)
    .catch(unhandledError)
}
