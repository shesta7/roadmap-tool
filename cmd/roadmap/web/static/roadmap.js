(() => {
  const $ = (selector) => document.querySelector(selector);
  const root = document.documentElement;
  const timeline = $('#timeline');
  const milestones = $('#milestones');
  const errorBox = $('#error');
  const notice = $('#notice');
  const dialog = $('#settingsDialog');
  const form = $('#settingsForm');
  const colorNames = ['background', 'surface', 'text', 'muted', 'grid', 'feature', 'milestone', 'closed', 'future'];
  let config;
  let connected = false;
  let themeMode = localStorage.getItem('roadmap-theme') || 'auto';
  let showIssuePoints = localStorage.getItem('roadmap-issue-points') !== 'hidden';
  let showMonths = localStorage.getItem('roadmap-months') !== 'hidden';

  async function request(url, options) {
    const response = await fetch(url, options);
    if (!response.ok) throw new Error((await response.text()).trim() || `Request failed (${response.status})`);
    const type = response.headers.get('content-type') || '';
    return type.includes('json') ? response.json() : null;
  }

  function actualTheme() {
    return themeMode === 'auto'
      ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : themeMode;
  }

  function applyTheme() {
    if (!config) return;
    const colors = config.theme[actualTheme()];
    for (const [name, value] of Object.entries(colors)) root.style.setProperty(`--${name}`, value);
    root.dataset.theme = actualTheme();
    $('#themeBtn').textContent = `Theme: ${themeMode}`;
  }

  function setConnected(value) {
    connected = value;
    const label = $('#connection');
    label.textContent = connected ? 'Connected' : 'Not connected';
    label.classList.toggle('online', connected);
    $('#disconnectBtn').disabled = !connected;
  }

  function date(value) {
    return new Date(value).toLocaleDateString(undefined, {day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC'});
  }

  function isoDate(value) {
    return new Date(value).toISOString().slice(0, 10);
  }

  function link(text, href, className) {
    const a = document.createElement('a');
    a.textContent = text;
    a.href = href;
    a.className = className || '';
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    return a;
  }

  function renderTimeline(data) {
    timeline.replaceChildren();
    data.Milestones ||= [];
    if (!data.Milestones.length) {
      const empty = document.createElement('p');
      empty.className = 'empty';
      empty.textContent = 'No dated milestones found.';
      timeline.append(empty);
      return;
    }
    const range = document.createElement('div');
    range.className = 'timeline-range';
    const track = document.createElement('div');
    track.className = 'timeline-track';
    track.style.minWidth = `${Math.max(760, data.Milestones.length * 220)}px`;
    const monthLayer = document.createElement('div');
    monthLayer.className = 'month-layer';
    const progress = document.createElement('div');
    progress.className = 'timeline-progress';
    track.append(monthLayer, progress);

    let currentAssigned = false;
    for (const [index, item] of data.Milestones.entries()) {
      const closed = item.State.toLowerCase() === 'closed';
      const status = closed ? 'closed' : currentAssigned ? 'future' : 'current';
      item._timelineStatus = status;
      if (!closed) currentAssigned = true;
      const point = document.createElement('div');
      point.className = `timeline-point ${index % 2 ? 'below' : 'above'} ${status}`;
      point.dataset.milestoneId = item.ID;
      point.dataset.timestamp = new Date(item.Due).getTime();
      const title = link(item.Title, item.URL, 'point-title');
      title.title = 'Open milestone in Git';
      const description = document.createElement('span');
      description.className = 'point-description';
      description.textContent = item.Description || 'No description';
      const due = document.createElement('span');
      due.className = 'point-date';
      due.textContent = date(item.Due);
      const dot = document.createElement('button');
      dot.className = 'point-dot';
      dot.type = 'button';
      dot.title = `Show ${item.Title} details`;
      dot.addEventListener('click', () => document.getElementById(`milestone-${item.ID}`).scrollIntoView({behavior: 'smooth'}));
      point.append(title, description, due, dot);
      track.append(point);

      for (const issue of item.Issues || []) {
        if (!issue.DueDate) continue;
        const issuePoint = link('', issue.URL, 'issue-point');
        issuePoint.dataset.milestoneId = item.ID;
        issuePoint.dataset.issueId = issue.ID;
        issuePoint.dataset.timestamp = new Date(issue.DueDate).getTime();
        issuePoint.dataset.tooltip = `#${issue.ID} ${issue.Title} · ${date(issue.DueDate)}`;
        issuePoint.setAttribute('aria-label', issuePoint.dataset.tooltip);
        track.append(issuePoint);
      }
    }
    timeline.append(range, track);
    $('#dateFrom').value = isoDate(data.MinDate);
    $('#dateTo').value = isoDate(data.MaxDate);
    $('#dateFrom').max = $('#dateTo').value;
    $('#dateTo').min = $('#dateFrom').value;
    layoutTimeline();
  }

  function layoutTimeline() {
    const from = $('#dateFrom').value;
    const to = $('#dateTo').value;
    if (!from || !to) return;
    const start = new Date(`${from}T00:00:00Z`).getTime();
    const end = new Date(`${to}T23:59:59Z`).getTime();
    const span = Math.max(1, end - start);
    for (const point of timeline.querySelectorAll('[data-timestamp]')) {
      const raw = 100 * (Number(point.dataset.timestamp) - start) / span;
      point.style.left = `${Math.max(3, Math.min(97, raw))}%`;
      point.classList.toggle('left-edge', raw < 10);
      point.classList.toggle('right-edge', raw > 90);
    }
    const current = timeline.querySelector('.timeline-point.current');
    const progress = timeline.querySelector('.timeline-progress');
    if (progress) {
      const raw = current ? 100 * (Number(current.dataset.timestamp) - start) / span : 100;
      progress.style.width = `${Math.max(0, Math.min(100, raw))}%`;
    }
    const range = timeline.querySelector('.timeline-range');
    if (range) range.textContent = `${date(`${from}T00:00:00Z`)} — ${date(`${to}T00:00:00Z`)}`;
    renderMonths(start, end, span);
    updateViewButtons();
  }

  function renderMonths(start, end, span) {
    const layer = timeline.querySelector('.month-layer');
    if (!layer) return;
    layer.replaceChildren();
    layer.hidden = !showMonths;
    const startDate = new Date(start);
    let current = Date.UTC(startDate.getUTCFullYear(), startDate.getUTCMonth(), 1);
    if (current < start) current = Date.UTC(startDate.getUTCFullYear(), startDate.getUTCMonth() + 1, 1);
    while (current <= end) {
      const marker = document.createElement('span');
      const markerDate = new Date(current);
      marker.className = 'month-marker';
      marker.style.left = `${100 * (current - start) / span}%`;
      marker.textContent = markerDate.toLocaleDateString(undefined, {month: 'short', year: 'numeric', timeZone: 'UTC'});
      layer.append(marker);
      current = Date.UTC(markerDate.getUTCFullYear(), markerDate.getUTCMonth() + 1, 1);
    }
  }

  function updateViewButtons() {
    $('#issuesToggleBtn').textContent = `Issue points: ${showIssuePoints ? 'shown' : 'hidden'}`;
    $('#monthsToggleBtn').textContent = `Months: ${showMonths ? 'shown' : 'hidden'}`;
  }

  function renderMilestones(data) {
    milestones.replaceChildren();
    for (const [index, item] of data.Milestones.entries()) {
      item.Issues ||= [];
      const details = document.createElement('details');
      details.className = `milestone-card ${item._timelineStatus || 'future'}`;
      details.id = `milestone-${item.ID}`;
      details.dataset.milestoneId = item.ID;
      details.dataset.due = isoDate(item.Due);
      details.dataset.summarySearch = `${item.Title} ${item.Description || ''}`.toLowerCase();
      details.open = index === 0;
      const summary = document.createElement('summary');
      const heading = document.createElement('div');
      heading.className = 'milestone-heading';
      heading.append(link(item.Title, item.URL, 'milestone-title'));
      const description = document.createElement('p');
      description.textContent = item.Description || 'No description';
      heading.append(description);
      const meta = document.createElement('div');
      meta.className = 'milestone-meta';
      const metaDate = document.createElement('strong');
      metaDate.textContent = date(item.Due);
      const issueCount = document.createElement('span');
      issueCount.textContent = `${item.Issues.length} issue${item.Issues.length === 1 ? '' : 's'}`;
      meta.append(metaDate, issueCount, link('Open milestone ↗', item.URL, 'git-link'));
      summary.append(heading, meta);

      const issueList = document.createElement('div');
      issueList.className = 'issue-list';
      if (!item.Issues.length) {
        const empty = document.createElement('p');
        empty.className = 'empty';
        empty.textContent = `No issues with label “${config.feature_label}”.`;
        issueList.append(empty);
      }
      for (const issue of item.Issues) {
        const row = document.createElement('div');
        row.className = 'issue-row';
        row.dataset.issueId = issue.ID;
        row.dataset.search = `${issue.ID} ${issue.Title} ${issue.State}`.toLowerCase();
        const title = document.createElement('div');
        title.append(link(`#${issue.ID} ${issue.Title}`, issue.URL, 'issue-title'));
        if (issue.DueDate) {
          const issueDue = document.createElement('span');
          issueDue.className = 'issue-due';
          issueDue.textContent = `Due ${date(issue.DueDate)}`;
          title.append(issueDue);
        }
        const state = document.createElement('span');
        state.className = `state ${issue.State.toLowerCase()}`;
        state.textContent = issue.State;
        row.append(title, state);
        issueList.append(row);
      }
      details.append(summary, issueList);
      details.addEventListener('toggle', updateDetailsButton);
      milestones.append(details);
    }
    applyFilter();
  }

  function visibleCards() {
    return [...milestones.querySelectorAll('.milestone-card')].filter((card) => !card.hidden);
  }

  function updateDetailsButton() {
    const cards = visibleCards();
    $('#detailsBtn').disabled = !cards.length;
    $('#detailsBtn').textContent = cards.some((card) => !card.open) ? 'Show all details' : 'Hide all details';
  }

  function applyFilter() {
    const query = $('#filterInput').value.trim().toLowerCase();
    const from = $('#dateFrom').value;
    const to = $('#dateTo').value;
    let matches = 0;
    for (const card of milestones.querySelectorAll('.milestone-card')) {
      const summaryMatches = card.dataset.summarySearch.includes(query);
      const rows = [...card.querySelectorAll('.issue-row')];
      const matchingRows = rows.filter((row) => row.dataset.search.includes(query));
      const inDateRange = (!from || card.dataset.due >= from) && (!to || card.dataset.due <= to);
      card.hidden = !inDateRange || (!!query && !summaryMatches && !matchingRows.length);
      if (!card.hidden) matches++;
      for (const row of rows) row.hidden = !!query && !summaryMatches && !row.dataset.search.includes(query);
      if (query && matchingRows.length && !summaryMatches) card.open = true;
      const points = timeline.querySelectorAll(`[data-milestone-id="${CSS.escape(card.dataset.milestoneId)}"]`);
      for (const point of points) {
        const issueID = point.dataset.issueId;
        const issueRow = issueID ? rows.find((row) => row.dataset.issueId === issueID) : null;
        const timestamp = Number(point.dataset.timestamp);
        const afterFrom = !from || timestamp >= new Date(`${from}T00:00:00Z`).getTime();
        const beforeTo = !to || timestamp <= new Date(`${to}T23:59:59Z`).getTime();
        point.hidden = card.hidden || !afterFrom || !beforeTo || !!(issueRow && issueRow.hidden) || (point.classList.contains('issue-point') && !showIssuePoints);
      }
    }
    $('#noResults').hidden = matches > 0;
    layoutTimeline();
    updateDetailsButton();
  }

  async function loadRoadmap() {
    errorBox.hidden = true;
    try {
      const data = await request('/api/roadmap');
      notice.hidden = !data.SkippedMilestones;
      notice.textContent = data.SkippedMilestones
        ? `${data.SkippedMilestones} milestone${data.SkippedMilestones === 1 ? '' : 's'} without a due date ${data.SkippedMilestones === 1 ? 'was' : 'were'} omitted.`
        : '';
      renderTimeline(data);
      renderMilestones(data);
    } catch (error) {
      errorBox.hidden = false;
      errorBox.textContent = error.message;
      timeline.replaceChildren();
      milestones.replaceChildren();
    }
  }

  function buildColors() {
    $('#colors').replaceChildren();
    for (const variant of ['light', 'dark']) {
      const group = document.createElement('div');
      group.className = 'color-group';
      const title = document.createElement('h3');
      title.textContent = variant[0].toUpperCase() + variant.slice(1);
      group.append(title);
      for (const name of colorNames) {
        const label = document.createElement('label');
        label.textContent = name[0].toUpperCase() + name.slice(1);
        const input = document.createElement('input');
        input.type = 'color';
        input.name = `${variant}_${name}`;
        input.value = config.theme[variant][name];
        label.append(input);
        group.append(label);
      }
      $('#colors').append(group);
    }
  }

  function fillForm() {
    for (const name of ['provider', 'base_url', 'owner', 'repository', 'feature_label']) form.elements[name].value = config[name] || '';
    form.elements.theme_default.value = config.theme.default;
    buildColors();
    $('#settingsMessage').textContent = '';
  }

  function configFromForm() {
    const next = {
      provider: form.elements.provider.value,
      base_url: form.elements.base_url.value.trim(),
      owner: form.elements.owner.value.trim(),
      repository: form.elements.repository.value.trim(),
      feature_label: form.elements.feature_label.value.trim(),
      theme: {default: form.elements.theme_default.value, light: {}, dark: {}}
    };
    for (const variant of ['light', 'dark']) {
      for (const name of colorNames) next.theme[variant][name] = form.elements[`${variant}_${name}`].value;
    }
    return next;
  }

  async function saveConfig(next) {
    await request('/api/config', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(next)});
    config = next;
    applyTheme();
    $('#subtitle').textContent = `${config.owner}/${config.repository} · ${config.provider}`;
    await loadRoadmap();
  }

  $('#themeBtn').addEventListener('click', () => {
    themeMode = themeMode === 'auto' ? 'light' : themeMode === 'light' ? 'dark' : 'auto';
    localStorage.setItem('roadmap-theme', themeMode);
    applyTheme();
  });
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme);
  $('#filterInput').addEventListener('input', applyFilter);
  $('#issuesToggleBtn').addEventListener('click', () => {
    showIssuePoints = !showIssuePoints;
    localStorage.setItem('roadmap-issue-points', showIssuePoints ? 'shown' : 'hidden');
    applyFilter();
  });
  $('#monthsToggleBtn').addEventListener('click', () => {
    showMonths = !showMonths;
    localStorage.setItem('roadmap-months', showMonths ? 'shown' : 'hidden');
    layoutTimeline();
  });
  for (const id of ['dateFrom', 'dateTo']) {
    $(`#${id}`).addEventListener('change', (event) => {
      const from = $('#dateFrom');
      const to = $('#dateTo');
      if (from.value && to.value && from.value > to.value) {
        if (event.target === from) to.value = from.value;
        else from.value = to.value;
      }
      from.max = to.value;
      to.min = from.value;
      applyFilter();
    });
  }
  $('#detailsBtn').addEventListener('click', () => {
    const cards = visibleCards();
    const open = cards.some((card) => !card.open);
    for (const card of cards) card.open = open;
    updateDetailsButton();
  });
  $('#settingsBtn').addEventListener('click', () => { fillForm(); dialog.showModal(); });
  $('#closeSettings').addEventListener('click', () => dialog.close());
  dialog.addEventListener('click', (event) => { if (event.target === dialog) dialog.close(); });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const message = $('#settingsMessage');
    try {
      await saveConfig(configFromForm());
      message.textContent = 'Settings saved.';
    } catch (error) { message.textContent = error.message; }
  });

  $('#connectBtn').addEventListener('click', async () => {
    const message = $('#settingsMessage');
    try {
      await request('/api/token', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token: $('#tokenInput').value})});
      $('#tokenInput').value = '';
      setConnected(true);
      message.textContent = 'Token saved.';
      await loadRoadmap();
    } catch (error) { message.textContent = error.message; }
  });

  $('#disconnectBtn').addEventListener('click', async () => {
    const message = $('#settingsMessage');
    try {
      await request('/api/token', {method: 'DELETE'});
      setConnected(false);
      message.textContent = 'Token removed.';
    } catch (error) { message.textContent = error.message; }
  });

  $('#importBtn').addEventListener('click', () => $('#importFile').click());
  $('#importFile').addEventListener('change', async (event) => {
    const message = $('#settingsMessage');
    try {
      const next = JSON.parse(await event.target.files[0].text());
      await saveConfig(next);
      fillForm();
      message.textContent = 'Configuration imported. The saved token was unchanged.';
    } catch (error) { message.textContent = `Import failed: ${error.message}`; }
    event.target.value = '';
  });

  request('/api/config').then((data) => {
    config = data.config;
    if (!localStorage.getItem('roadmap-theme')) themeMode = config.theme.default;
    setConnected(data.connected);
    applyTheme();
    $('#subtitle').textContent = `${config.owner}/${config.repository} · ${config.provider}`;
    return loadRoadmap();
  }).catch((error) => {
    errorBox.hidden = false;
    errorBox.textContent = error.message;
  });
})();
