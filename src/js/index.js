/**
 * JavaScript code for chouf
 * author: Nicolas CARPi
 * copyright: 2022
 * license: MIT
 * repo: https://github.com/deltablot/chouf
 */
import 'bootstrap/dist/css/bootstrap.min.css';
import '../css/main.css';

const updateSite = result => {
  const r = JSON.parse(result);
  const el = document.querySelector(`[data-domain="${r.domain}"]`);
  const status = r.ok ? 'ok' : 'ko';
  el.dataset.status = status;
};

document.addEventListener('DOMContentLoaded', () => {
  const ws = new WebSocket('ws://localhost:3003/ws');
  ws.onopen = () => console.debug('websocket ready');
  ws.onmessage = event => {
    updateSite(event.data);
  };
  const btn = document.createElement('button')
  btn.id = 'btn'
  btn.addEventListener('click', e => {
    ws.close();
  });
  btn.innerText = 'try close';
  const footer = document.getElementById('f');
  footer.append(btn);

  const btnMsg = document.createElement('button')
  btnMsg.id = 'btnMsg'
  btnMsg.addEventListener('click', e => {
    ws.send('test');
  });
  btnMsg.innerText = 'try send';
  footer.append(btn);
  footer.append(btnMsg);
  // make sure the socket is closed when client quits
  window.onbeforeunload = () => {
    ws.close(1000, 'test');
  };
});
