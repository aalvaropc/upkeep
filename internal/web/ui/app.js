"use strict";

(() => {
  const session = document.body.dataset.session || "local";
  const status = document.getElementById("status");
  const list = document.getElementById("events");
  const maxItems = 50;

  const setStatus = (state, text) => {
    status.dataset.state = state;
    status.textContent = text;
  };

  const log = (text) => {
    const item = document.createElement("li");
    item.textContent = `${new Date().toLocaleTimeString()}  ${text}`;
    list.prepend(item);
    while (list.children.length > maxItems) {
      list.lastElementChild.remove();
    }
  };

  const source = new EventSource(`/api/sessions/${encodeURIComponent(session)}/events`);

  source.addEventListener("hello", (e) => {
    const hello = JSON.parse(e.data);
    setStatus("live", `live · ${hello.version || "dev"}`);
    log(`connected to session ${hello.session}`);
  });

  source.addEventListener("ping", () => {
    if (status.dataset.state !== "live") {
      setStatus("live", "live");
    }
  });

  source.addEventListener("gap", () => {
    log("some events were missed; showing the latest ones");
  });

  source.onmessage = (e) => {
    const event = JSON.parse(e.data);
    log(`#${event.seq} ${event.kind}${event.pr ? ` · PR ${event.pr}` : ""}`);
  };

  source.onerror = () => {
    setStatus("lost", "reconnecting");
  };
})();
