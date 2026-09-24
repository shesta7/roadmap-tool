(() => {
  const $ = (selector) => document.querySelector(selector);
  const root = document.documentElement;
  const timeline = $('#timeline');
  const detailsPanel = $('#milestoneDetails');
  const errorBox = $('#error');
  const notice = $('#notice');
  const dialog = $('#settingsDialog');
  const form = $('#settingsForm');
  const colorNames = ['background', 'surface', 'text', 'muted', 'grid', 'feature', 'milestone', 'current', 'closed', 'future'];
  const colorLabels = {feature: 'Feature points', milestone: 'Selected milestone', current: 'Current milestone', closed: 'Completed milestone', future: 'Future milestone'};
  const appearanceFields = ['timeline_width', 'timeline_height', 'milestone_size', 'selected_ring', 'label_width', 'label_font_size', 'feature_row_height'];
  let library;
  let connectionStatus = {};
  let draftLibrary;
  let editingProjectID;
  let editingConnectionID;
  let roadmapData;
  let selectedMilestoneID;
  let themeMode = localStorage.getItem('roadmap-theme') || 'auto';
  let overviewMode = localStorage.getItem('roadmap-overview-mode-v1') === 'shown';
  let showIssuePoints = localStorage.getItem('roadmap-feature-points-v2') === 'shown';
  let showMonths = localStorage.getItem('roadmap-month-scale-v2') !== 'hidden';

  async function request(url, options) {
    const response = await fetch(url, options);
    if (!response.ok) throw new Error((await response.text()).trim() || `Request failed (${response.status})`);
    const type = response.headers.get('content-type') || '';
    return type.includes('json') ? response.json() : null;
  }

  const clone = (value) => JSON.parse(JSON.stringify(value));
  const makeID = (prefix) => `${prefix}-${crypto.randomUUID()}`;
  const projectByID = (source, id) => source.projects.find((project) => project.id === id);
  const connectionByID = (source, id) => source.connections.find((connection) => connection.id === id);
  const activeProject = () => projectByID(library, library.active_project_id);
  const activeConnection = () => connectionByID(library, activeProject().connection_id);
  const timestamp = (value) => new Date(value).getTime();
  const isoDate = (value) => new Date(value).toISOString().slice(0, 10);

  function actualTheme() {
    return themeMode === 'auto'
      ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : themeMode;
  }

  function applyTheme() {
    if (!library) return;
    const colors = activeProject().theme[actualTheme()];
    for (const [name, value] of Object.entries(colors)) root.style.setProperty(`--${name}`, value);
    const appearance = activeProject().appearance;
    root.style.setProperty('--timeline-width', `${appearance.timeline_width}px`);
    root.style.setProperty('--timeline-height', `${appearance.timeline_height}px`);
    root.style.setProperty('--timeline-axis-y', `${Math.round(appearance.timeline_height / 2)}px`);
    root.style.setProperty('--milestone-size', `${appearance.milestone_size}px`);
    root.style.setProperty('--selected-ring', `${appearance.selected_ring}px`);
    root.style.setProperty('--label-width', `${appearance.label_width}px`);
    root.style.setProperty('--label-font-size', `${appearance.label_font_size}px`);
    root.style.setProperty('--feature-row-height', `${appearance.feature_row_height}px`);
    root.dataset.theme = actualTheme();
    const themeButton = $('#themeBtn');
    const themeLabel = `Theme: ${themeMode}`;
    themeButton.setAttribute('aria-label', themeLabel);
    themeButton.title = themeLabel;
    themeButton.dataset.mode = themeMode;
  }

  function setConnected(value) {
    const label = $('#connection');
    label.textContent = value ? 'Connected' : 'Not connected';
    label.classList.toggle('online', value);
  }

  function renderProjectSwitcher() {
    const select = $('#projectSwitcher');
    select.replaceChildren();
    for (const project of library.projects) {
      const option = document.createElement('option');
      option.value = project.id;
      option.textContent = project.name;
      select.append(option);
    }
    select.value = library.active_project_id;
    const project = activeProject();
    const connection = activeConnection();
    $('#subtitle').textContent = `${project.owner}/${project.repository} · ${connection.provider}`;
    setConnected(!!connectionStatus[connection.id]);
  }

  function date(value) {
    return new Date(value).toLocaleDateString(undefined, {day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC'});
  }

  function shortDate(value) {
    return new Date(value).toLocaleDateString(undefined, {day: 'numeric', month: 'short', timeZone: 'UTC'});
  }

  function month(value) {
    return new Date(value).toLocaleDateString(undefined, {month: 'short', year: 'numeric', timeZone: 'UTC'});
  }

  function link(text, href, className) {
    const anchor = document.createElement('a');
    anchor.textContent = text;
    anchor.href = href;
    anchor.className = className || '';
    anchor.target = '_blank';
    anchor.rel = 'noopener noreferrer';
    return anchor;
  }

  function lifecycleStatuses(items) {
    let currentAssigned = false;
    return new Map(items.map((item) => {
      const closed = item.State.toLowerCase() === 'closed';
      const status = closed ? 'closed' : currentAssigned ? 'future' : 'current';
      if (!closed) currentAssigned = true;
      return [item.ID, status];
    }));
  }

  function defaultViewport(data) {
    const anchor = data.Milestones.find((item) => item.State.toLowerCase() !== 'closed') || data.Milestones.at(-1);
    const due = anchor ? new Date(anchor.Due) : new Date();
    const from = new Date(Date.UTC(due.getUTCFullYear(), due.getUTCMonth() - 4, 1));
    const to = new Date(Date.UTC(due.getUTCFullYear(), due.getUTCMonth() + 2, 0));
    $('#dateFrom').value = isoDate(from);
    $('#dateTo').value = isoDate(to);
    $('#dateFrom').max = $('#dateTo').value;
    $('#dateTo').min = $('#dateFrom').value;
  }

  function currentRange() {
    const from = $('#dateFrom').value;
    const to = $('#dateTo').value;
    const start = timestamp(`${from}T00:00:00Z`);
    const end = timestamp(`${to}T23:59:59Z`);
    return {from, to, start, end, span: Math.max(1, end - start)};
  }

  function matchesQuery(item, query) {
    if (!query) return true;
    const milestoneText = `${item.Title} ${item.Description || ''}`.toLowerCase();
    return milestoneText.includes(query) || (item.Issues || []).some((issue) =>
      `${issue.ID} ${issue.Title} ${issue.State}`.toLowerCase().includes(query));
  }

  function renderMonths(layer, range) {
    layer.hidden = !showMonths;
    if (!showMonths) return;
    const first = new Date(range.start);
    let cursor = Date.UTC(first.getUTCFullYear(), first.getUTCMonth(), 1);
    if (cursor < range.start) cursor = Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 1);
    while (cursor <= range.end) {
      const marker = document.createElement('span');
      const position = 100 * (cursor - range.start) / range.span;
      const edge = position < 4 ? ' edge-left' : position > 96 ? ' edge-right' : '';
      marker.className = `month-marker${edge}`;
      marker.style.left = `${position}%`;
      marker.textContent = new Date(cursor).toLocaleDateString(undefined, {month: 'short', timeZone: 'UTC'});
      layer.append(marker);
      const value = new Date(cursor);
      cursor = Date.UTC(value.getUTCFullYear(), value.getUTCMonth() + 1, 1);
    }
  }

  function renderTimeline() {
    timeline.replaceChildren();
    if (!roadmapData?.Milestones?.length) {
      const empty = document.createElement('p');
      empty.className = 'empty';
      empty.textContent = 'No dated milestones found.';
      timeline.append(empty);
      $('#noResults').hidden = true;
      return;
    }

    const range = currentRange();
    const query = $('#filterInput').value.trim().toLowerCase();
    const statuses = lifecycleStatuses(roadmapData.Milestones);
    const visible = roadmapData.Milestones.filter((item) => {
      const due = timestamp(item.Due);
      return due >= range.start && due <= range.end && matchesQuery(item, query);
    });
    $('#noResults').hidden = visible.length > 0;
    $('#rangeLabel').textContent = `${month(`${range.from}T00:00:00Z`)} — ${month(`${range.to}T00:00:00Z`)}`;

    const track = document.createElement('div');
    track.className = 'timeline-track';
    const monthLayer = document.createElement('div');
    monthLayer.className = 'month-layer';
    renderMonths(monthLayer, range);
    track.append(monthLayer);
    let selectedPosition = null;

    for (const [index, item] of visible.entries()) {
      const position = 100 * (timestamp(item.Due) - range.start) / range.span;
      const node = document.createElement('button');
      const selected = !overviewMode && item.ID === selectedMilestoneID;
      if (selected) selectedPosition = position;
      node.type = 'button';
      const edge = position < 18 ? ' edge-left' : position > 82 ? ' edge-right' : '';
      const stateClass = overviewMode ? 'overview' : statuses.get(item.ID);
      node.className = `timeline-node ${index % 2 ? 'below' : 'above'} ${stateClass}${selected ? ' selected' : ''}${edge}`;
      node.style.left = `${Math.max(2, Math.min(98, position))}%`;
      node.setAttribute('aria-expanded', String(selected));
      node.setAttribute('aria-controls', 'milestoneDetails');
      node.title = overviewMode
        ? `Show ${item.Title} details`
        : selected ? `Hide ${item.Title} details` : `Show ${item.Title} details`;
      const label = document.createElement('span');
      label.className = 'node-label';
      const title = document.createElement('strong');
      title.textContent = item.Title;
      const due = document.createElement('span');
      due.className = 'node-date';
      due.textContent = shortDate(item.Due);
      const description = document.createElement('span');
      description.className = 'node-description';
      description.textContent = item.Description || 'No description';
      label.append(title, due, description);
      const dot = document.createElement('span');
      dot.className = 'node-dot';
      node.append(label, dot);
      node.addEventListener('click', () => {
        if (overviewMode) {
          overviewMode = false;
          $('#overviewToggle').checked = false;
          localStorage.setItem('roadmap-overview-mode-v1', 'hidden');
          selectedMilestoneID = item.ID;
        } else {
          selectedMilestoneID = selectedMilestoneID === item.ID ? null : item.ID;
        }
        renderTimeline();
        renderDetails();
      });
      track.append(node);

      if (showIssuePoints) {
        for (const issue of item.Issues || []) {
          if (!issue.DueDate) continue;
          const issueDue = timestamp(issue.DueDate);
          if (issueDue < range.start || issueDue > range.end) continue;
          const issuePoint = link('', issue.URL, 'issue-point');
          issuePoint.style.left = `${100 * (issueDue - range.start) / range.span}%`;
          issuePoint.dataset.tooltip = `#${issue.ID} ${issue.Title}`;
          issuePoint.setAttribute('aria-label', issuePoint.dataset.tooltip);
          track.append(issuePoint);
        }
      }
    }
    timeline.append(track);
    if (selectedPosition !== null) {
      requestAnimationFrame(() => {
        const target = track.scrollWidth * selectedPosition / 100 - timeline.clientWidth * .66;
        timeline.scrollLeft = Math.max(0, target);
      });
    }
  }

  function renderDetails() {
    detailsPanel.replaceChildren();
    if (overviewMode) {
      detailsPanel.hidden = true;
      return;
    }
    const item = roadmapData?.Milestones?.find((milestone) => milestone.ID === selectedMilestoneID);
    const range = currentRange();
    const itemDue = item ? timestamp(item.Due) : 0;
    if (!item || itemDue < range.start || itemDue > range.end || !matchesQuery(item, $('#filterInput').value.trim().toLowerCase())) {
      detailsPanel.hidden = true;
      return;
    }
    detailsPanel.hidden = false;
    const header = document.createElement('header');
    header.className = 'detail-header';
    const heading = document.createElement('div');
    heading.append(link(item.Title, item.URL, 'detail-title'));
    const description = document.createElement('p');
    description.textContent = item.Description || 'No description';
    heading.append(description);
    const meta = document.createElement('div');
    meta.className = 'detail-meta';
    const due = document.createElement('strong');
    due.textContent = date(item.Due);
    const count = document.createElement('span');
    count.textContent = `${(item.Issues || []).length} feature${item.Issues?.length === 1 ? '' : 's'}`;
    meta.append(due, count, link('Open in Git ↗', item.URL, 'git-link'));
    header.append(heading, meta);

    const list = document.createElement('div');
    list.className = 'feature-list';
    const query = $('#filterInput').value.trim().toLowerCase();
    const summaryMatches = `${item.Title} ${item.Description || ''}`.toLowerCase().includes(query);
    const features = (item.Issues || []).filter((issue) => !query || summaryMatches ||
      `${issue.ID} ${issue.Title} ${issue.State}`.toLowerCase().includes(query));
    if (!features.length) {
      const empty = document.createElement('p');
      empty.className = 'empty';
      empty.textContent = `No features with label “${activeProject().feature_label}”.`;
      list.append(empty);
    }
    for (const issue of features) {
      const row = document.createElement('div');
      row.className = 'feature-row';
      const closed = issue.State.toLowerCase() === 'closed';
      const icon = document.createElement('span');
      icon.className = `feature-status-icon ${closed ? 'closed' : 'open'}`;
      icon.setAttribute('aria-label', closed ? 'Closed' : 'Open');
      const issueID = link(`#${issue.ID}`, issue.URL, 'feature-id');
      const title = link(issue.Title, issue.URL, 'feature-title');
      const state = document.createElement('span');
      state.className = `state ${issue.State.toLowerCase()}`;
      state.textContent = issue.State;
      row.append(icon, issueID, title, state);
      list.append(row);
    }
    detailsPanel.append(header, list);
  }

  function renderRoadmap() {
    renderTimeline();
    renderDetails();
  }

  async function loadRoadmap() {
    errorBox.hidden = true;
    notice.hidden = true;
    try {
      roadmapData = await request('/api/roadmap');
      roadmapData.Milestones ||= [];
      for (const item of roadmapData.Milestones) item.Issues ||= [];
      notice.hidden = !roadmapData.SkippedMilestones;
      notice.textContent = roadmapData.SkippedMilestones
        ? `${roadmapData.SkippedMilestones} milestone${roadmapData.SkippedMilestones === 1 ? '' : 's'} without a due date ${roadmapData.SkippedMilestones === 1 ? 'was' : 'were'} omitted.`
        : '';
      selectedMilestoneID = overviewMode
        ? null
        : (roadmapData.Milestones.find((item) => item.State.toLowerCase() !== 'closed') || roadmapData.Milestones.at(-1))?.ID || null;
      defaultViewport(roadmapData);
      renderRoadmap();
    } catch (error) {
      errorBox.hidden = false;
      errorBox.textContent = error.message;
      timeline.replaceChildren();
      detailsPanel.hidden = true;
    }
  }

  async function fetchConfiguration() {
    const data = await request('/api/config');
    library = data.config;
    connectionStatus = data.connection_status;
    renderProjectSwitcher();
    applyTheme();
  }

  async function persistLibrary(next) {
    const nextConnectionIDs = new Set(next.connections.map((connection) => connection.id));
    const removedConnectionIDs = library
      ? library.connections.filter((connection) => !nextConnectionIDs.has(connection.id)).map((connection) => connection.id)
      : [];
    await request('/api/config', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(next)});
    await Promise.all(removedConnectionIDs.map((connectionID) =>
      request(`/api/token?connection_id=${encodeURIComponent(connectionID)}`, {method: 'DELETE'})));
    await fetchConfiguration();
  }

  function buildColors(theme) {
    $('#colors').replaceChildren();
    for (const variant of ['light', 'dark']) {
      const group = document.createElement('div');
      group.className = 'color-group';
      const title = document.createElement('h3');
      title.textContent = variant[0].toUpperCase() + variant.slice(1);
      group.append(title);
      for (const name of colorNames) {
        const label = document.createElement('label');
        label.textContent = colorLabels[name] || name[0].toUpperCase() + name.slice(1);
        const input = document.createElement('input');
        input.type = 'color';
        input.name = `${variant}_${name}`;
        input.value = theme[variant][name];
        label.append(input);
        group.append(label);
      }
      $('#colors').append(group);
    }
  }

  function saveVisibleForm() {
    if (!draftLibrary || !editingProjectID) return;
    const project = projectByID(draftLibrary, editingProjectID);
    if (!project) return;
    project.name = form.elements.project_name.value.trim();
    project.owner = form.elements.owner.value.trim();
    project.repository = form.elements.repository.value.trim();
    project.feature_label = form.elements.feature_label.value.trim();
    project.connection_id = editingConnectionID;
    project.theme.default = form.elements.theme_default.value;
    for (const name of appearanceFields) project.appearance[name] = Number.parseInt(form.elements[name].value, 10);
    for (const variant of ['light', 'dark']) {
      for (const name of colorNames) project.theme[variant][name] = form.elements[`${variant}_${name}`].value;
    }
    const connection = connectionByID(draftLibrary, editingConnectionID);
    connection.name = form.elements.connection_name.value.trim();
    connection.provider = form.elements.provider.value;
    connection.base_url = form.elements.base_url.value.trim();
  }

  function renderLibraryOptions() {
    const profileSelect = $('#profileSelect');
    profileSelect.replaceChildren();
    for (const project of draftLibrary.projects) {
      const option = document.createElement('option');
      option.value = project.id;
      option.textContent = project.name;
      profileSelect.append(option);
    }
    profileSelect.value = editingProjectID;
    const connectionSelect = form.elements.connection_id;
    connectionSelect.replaceChildren();
    for (const connection of draftLibrary.connections) {
      const option = document.createElement('option');
      option.value = connection.id;
      option.textContent = connection.name;
      connectionSelect.append(option);
    }
  }

  function updateTokenControls(connectionID) {
    const connected = !!connectionStatus[connectionID];
    const saved = !!connectionByID(library, connectionID);
    $('#connectBtn').disabled = !saved;
    $('#disconnectBtn').disabled = !saved || !connected;
    $('#connectBtn').textContent = connected ? 'Replace token' : 'Save token';
  }

  function fillForm() {
    const project = projectByID(draftLibrary, editingProjectID);
    editingConnectionID = project.connection_id;
    renderLibraryOptions();
    form.elements.project_name.value = project.name;
    form.elements.connection_id.value = project.connection_id;
    form.elements.owner.value = project.owner;
    form.elements.repository.value = project.repository;
    form.elements.feature_label.value = project.feature_label;
    form.elements.theme_default.value = project.theme.default;
    for (const name of appearanceFields) form.elements[name].value = project.appearance[name];
    const connection = connectionByID(draftLibrary, project.connection_id);
    form.elements.connection_name.value = connection.name;
    form.elements.provider.value = connection.provider;
    form.elements.base_url.value = connection.base_url || '';
    buildColors(project.theme);
    updateTokenControls(connection.id);
    $('#deleteProfileBtn').disabled = draftLibrary.projects.length === 1;
    $('#deleteConnectionBtn').disabled = draftLibrary.connections.length === 1;
    $('#settingsMessage').textContent = '';
  }

  function openSettings() {
    draftLibrary = clone(library);
    editingProjectID = library.active_project_id;
    fillForm();
    dialog.showModal();
  }

  $('#projectSwitcher').addEventListener('change', async (event) => {
    const previous = library.active_project_id;
    const next = clone(library);
    next.active_project_id = event.target.value;
    try {
      await persistLibrary(next);
      $('#filterInput').value = '';
      await loadRoadmap();
    } catch (error) {
      event.target.value = previous;
      errorBox.hidden = false;
      errorBox.textContent = error.message;
    }
  });

  $('#themeBtn').addEventListener('click', () => {
    themeMode = themeMode === 'auto' ? 'light' : themeMode === 'light' ? 'dark' : 'auto';
    localStorage.setItem('roadmap-theme', themeMode);
    applyTheme();
  });
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme);
  $('#filterInput').addEventListener('input', renderRoadmap);
  $('#overviewToggle').checked = overviewMode;
  $('#issuesToggle').checked = showIssuePoints;
  $('#monthsToggle').checked = showMonths;
  $('#overviewToggle').addEventListener('change', (event) => {
    overviewMode = event.target.checked;
    localStorage.setItem('roadmap-overview-mode-v1', overviewMode ? 'shown' : 'hidden');
    selectedMilestoneID = overviewMode
      ? null
      : (roadmapData?.Milestones?.find((item) => item.State.toLowerCase() !== 'closed') || roadmapData?.Milestones?.at(-1))?.ID || null;
    renderRoadmap();
  });
  $('#issuesToggle').addEventListener('change', (event) => {
    showIssuePoints = event.target.checked;
    localStorage.setItem('roadmap-feature-points-v2', showIssuePoints ? 'shown' : 'hidden');
    renderTimeline();
  });
  $('#monthsToggle').addEventListener('change', (event) => {
    showMonths = event.target.checked;
    localStorage.setItem('roadmap-month-scale-v2', showMonths ? 'shown' : 'hidden');
    renderTimeline();
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
      renderRoadmap();
    });
  }

  $('#settingsBtn').addEventListener('click', openSettings);
  $('#closeSettings').addEventListener('click', () => dialog.close());
  dialog.addEventListener('click', (event) => { if (event.target === dialog) dialog.close(); });
  $('#profileSelect').addEventListener('change', (event) => {
    saveVisibleForm();
    editingProjectID = event.target.value;
    fillForm();
  });
  form.elements.connection_id.addEventListener('change', (event) => {
    saveVisibleForm();
    const project = projectByID(draftLibrary, editingProjectID);
    project.connection_id = event.target.value;
    editingConnectionID = event.target.value;
    fillForm();
  });

  $('#newProfileBtn').addEventListener('click', () => {
    saveVisibleForm();
    const project = clone(projectByID(draftLibrary, editingProjectID));
    project.id = makeID('project');
    project.name = 'New project';
    project.owner = '';
    project.repository = '';
    draftLibrary.projects.push(project);
    editingProjectID = project.id;
    fillForm();
    form.elements.project_name.select();
  });
  $('#duplicateProfileBtn').addEventListener('click', () => {
    saveVisibleForm();
    const project = clone(projectByID(draftLibrary, editingProjectID));
    project.id = makeID('project');
    project.name = `${project.name} copy`;
    draftLibrary.projects.push(project);
    editingProjectID = project.id;
    fillForm();
  });
  $('#deleteProfileBtn').addEventListener('click', () => {
    if (draftLibrary.projects.length === 1 || !confirm('Delete this project profile?')) return;
    draftLibrary.projects = draftLibrary.projects.filter((project) => project.id !== editingProjectID);
    editingProjectID = draftLibrary.projects[0].id;
    if (!projectByID(draftLibrary, draftLibrary.active_project_id)) draftLibrary.active_project_id = editingProjectID;
    fillForm();
  });
  $('#newConnectionBtn').addEventListener('click', () => {
    saveVisibleForm();
    const connection = {id: makeID('connection'), name: 'New connection', provider: 'github', base_url: ''};
    draftLibrary.connections.push(connection);
    projectByID(draftLibrary, editingProjectID).connection_id = connection.id;
    fillForm();
    form.elements.connection_name.select();
  });
  $('#deleteConnectionBtn').addEventListener('click', () => {
    saveVisibleForm();
    const project = projectByID(draftLibrary, editingProjectID);
    const id = project.connection_id;
    const usedElsewhere = draftLibrary.projects.some((item) => item.id !== project.id && item.connection_id === id);
    if (usedElsewhere) {
      $('#settingsMessage').textContent = 'This connection is used by another project profile.';
      return;
    }
    if (draftLibrary.connections.length === 1 || !confirm('Delete this connection? Its saved token will no longer be used.')) return;
    draftLibrary.connections = draftLibrary.connections.filter((connection) => connection.id !== id);
    project.connection_id = draftLibrary.connections[0].id;
    fillForm();
  });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const message = $('#settingsMessage');
    try {
      saveVisibleForm();
      draftLibrary.active_project_id = editingProjectID;
      await persistLibrary(draftLibrary);
      draftLibrary = clone(library);
      editingProjectID = library.active_project_id;
      fillForm();
      message.textContent = 'Library saved.';
      await loadRoadmap();
    } catch (error) { message.textContent = error.message; }
  });
  $('#connectBtn').addEventListener('click', async () => {
    const message = $('#settingsMessage');
    try {
      saveVisibleForm();
      const connectionID = projectByID(draftLibrary, editingProjectID).connection_id;
      await request('/api/token', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({connection_id: connectionID, token: $('#tokenInput').value})});
      $('#tokenInput').value = '';
      connectionStatus[connectionID] = true;
      updateTokenControls(connectionID);
      if (connectionID === activeConnection().id) setConnected(true);
      message.textContent = 'Token saved for this connection.';
    } catch (error) { message.textContent = error.message; }
  });
  $('#disconnectBtn').addEventListener('click', async () => {
    const message = $('#settingsMessage');
    try {
      const connectionID = projectByID(draftLibrary, editingProjectID).connection_id;
      await request(`/api/token?connection_id=${encodeURIComponent(connectionID)}`, {method: 'DELETE'});
      connectionStatus[connectionID] = false;
      updateTokenControls(connectionID);
      if (connectionID === activeConnection().id) setConnected(false);
      message.textContent = 'Token removed from this connection.';
    } catch (error) { message.textContent = error.message; }
  });
  $('#importBtn').addEventListener('click', () => $('#importFile').click());
  $('#importFile').addEventListener('change', async (event) => {
    const message = $('#settingsMessage');
    try {
      const next = JSON.parse(await event.target.files[0].text());
      await persistLibrary(next);
      draftLibrary = clone(library);
      editingProjectID = library.active_project_id;
      fillForm();
      message.textContent = 'Library imported. Tokens for retained connections were unchanged.';
      await loadRoadmap();
    } catch (error) { message.textContent = `Import failed: ${error.message}`; }
    event.target.value = '';
  });

  fetchConfiguration().then(() => {
    if (!localStorage.getItem('roadmap-theme')) themeMode = activeProject().theme.default;
    applyTheme();
    return loadRoadmap();
  }).catch((error) => {
    errorBox.hidden = false;
    errorBox.textContent = error.message;
  });
})();
