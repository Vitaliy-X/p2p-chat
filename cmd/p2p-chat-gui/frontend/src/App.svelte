<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, onEvent, type Message, type State } from './api';

  let username = 'alice';
  let room = 'local';
  let roomKey = '';
  let dbPath = 'p2p-chat-gui.db';
  let useDHT = true;
  let manualAddr = '';
  let draft = '';
  let messages: Message[] = [];
  let state: State = {
    connected: false,
    status: 'disconnected',
    peer_count: 0,
    room_peer_count: 0,
    peer_info: { id: '', addresses: [] },
  };
  let busy = false;
  let error = '';
  let copied = false;
  let messageList: HTMLDivElement;

  const statusText = () => {
    if (state.connected) return 'Connected';
    if (state.status === 'connecting') return 'Connecting';
    if (state.status === 'error') return 'Error';
    return 'Disconnected';
  };

  function applyState(next: State) {
    state = next;
    if (next.messages) {
      messages = next.messages;
      scrollToEnd();
    }
    error = next.last_error ?? '';
  }

  function addMessage(message: Message) {
    if (messages.some((item) => item.id === message.id)) return;
    messages = [...messages, message];
    scrollToEnd();
  }

  async function scrollToEnd() {
    await tick();
    if (messageList) {
      messageList.scrollTop = messageList.scrollHeight;
    }
  }

  async function connect() {
    busy = true;
    error = '';
    try {
      const next = await api().Connect({
        username,
        room,
        room_key: roomKey,
        db_path: dbPath,
        no_dht: !useDHT,
      });
      applyState(next);
    } catch (err) {
      error = String(err);
    } finally {
      busy = false;
    }
  }

  async function disconnect() {
    busy = true;
    error = '';
    try {
      await api().Disconnect();
      applyState(await api().Status());
      messages = [];
    } catch (err) {
      error = String(err);
    } finally {
      busy = false;
    }
  }

  async function send() {
    const text = draft.trim();
    if (!text || !state.connected) return;
    draft = '';
    error = '';
    try {
      await api().Send(text);
      error = '';
    } catch (err) {
      error = String(err);
    }
  }

  async function manualConnect() {
    const value = manualAddr.trim();
    if (!value) return;
    error = '';
    try {
      await api().ManualConnect(value);
      manualAddr = '';
      applyState(await api().Status());
      error = '';
    } catch (err) {
      error = String(err);
    }
  }

  async function copyPeerInfo() {
    if (!state.connected) return;
    error = '';
    try {
      await api().CopyPeerInfo();
      error = '';
      copied = true;
      setTimeout(() => (copied = false), 1200);
    } catch (err) {
      error = String(err);
    }
  }

  function formatTime(value: string) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  onMount(() => {
    api().Status().then(applyState).catch((err) => (error = String(err)));
    const offMessage = onEvent<Message>('chat:message', addMessage);
    const offStatus = onEvent<State>('chat:status', applyState);
    const statusTimer = window.setInterval(() => {
      if (!state.connected) return;
      api().Status().then(applyState).catch((err) => (error = String(err)));
    }, 2000);
    return () => {
      window.clearInterval(statusTimer);
      offMessage?.();
      offStatus?.();
    };
  });
</script>

<main class="app-shell">
  <aside class="sidebar">
    <div class="brand">
      <div>
        <h1>P2P Chat</h1>
        <p>{statusText()}</p>
      </div>
      <span class:online={state.connected}></span>
    </div>

    <section class="panel">
      <label>
        Username
        <input bind:value={username} disabled={state.connected || busy} autocomplete="off" />
      </label>

      <label>
        Room
        <input bind:value={room} disabled={state.connected || busy} autocomplete="off" />
      </label>

      <label>
        Room key
        <input
          type="password"
          bind:value={roomKey}
          disabled={state.connected || busy}
          autocomplete="off"
        />
      </label>

      <label>
        SQLite
        <input bind:value={dbPath} disabled={state.connected || busy} autocomplete="off" />
      </label>

      <label class="toggle">
        <input type="checkbox" bind:checked={useDHT} disabled={state.connected || busy} />
        <span>DHT discovery</span>
      </label>

      {#if state.connected}
        <button class="secondary" on:click={disconnect} disabled={busy}>Disconnect</button>
      {:else}
        <button class="primary" on:click={connect} disabled={busy}>Connect</button>
      {/if}
    </section>

    <section class="panel compact">
      <div class="metric">
        <span>Status</span>
        <strong>{state.status}</strong>
      </div>
      <div class="metric">
        <span>Peers</span>
        <strong>{state.peer_count}</strong>
      </div>
    </section>

    <section class="panel">
      <div class="peer-header">
        <span>Peer info</span>
        <button class="ghost" on:click={copyPeerInfo} disabled={!state.connected}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <textarea readonly value={state.peer_info?.addresses?.join('\n') ?? ''}></textarea>
    </section>

    <section class="panel">
      <label>
        Manual connect
        <input bind:value={manualAddr} disabled={!state.connected} autocomplete="off" />
      </label>
      <button class="secondary" on:click={manualConnect} disabled={!state.connected}>Connect peer</button>
    </section>

    {#if error}
      <div class="error">{error}</div>
    {/if}
  </aside>

  <section class="chat">
    <header class="chat-header">
      <div>
        <h2>{room || 'Room'}</h2>
        <p>{state.peer_info?.id || 'No local peer yet'}</p>
      </div>
      <div class="peer-count">{state.room_peer_count}</div>
    </header>

    <div class="messages" bind:this={messageList}>
      {#if messages.length === 0}
        <div class="empty">No messages</div>
      {:else}
        {#each messages as message (message.id)}
          <article class:mine={message.sender_username === username}>
            <div class="bubble">
              <div class="message-meta">
                <strong>{message.sender_username}</strong>
                <span>{formatTime(message.sent_at)}</span>
              </div>
              <p>{message.text}</p>
            </div>
          </article>
        {/each}
      {/if}
    </div>

    <form class="composer" on:submit|preventDefault={send}>
      <input
        bind:value={draft}
        disabled={!state.connected}
        placeholder={state.connected ? 'Message' : 'Connect first'}
        autocomplete="off"
      />
      <button class="primary" disabled={!state.connected || !draft.trim()}>Send</button>
    </form>
  </section>
</main>
