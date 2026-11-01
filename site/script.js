const snippets = {
  docker: `# Clone the repository\ngit clone https://github.com/sc0vu/funda-cli.git\ncd funda-cli\n\n# Create your local configuration\ncp examples/monitor.json monitor.json\nmkdir -p data\n\n# Configure your credentials, then launch\ndocker compose up -d --build`,
  go: `# Clone and build (Go 1.25+)\ngit clone https://github.com/sc0vu/funda-cli.git\ncd funda-cli\ngo build -o funda ./cmd/funda\n\n# Discover listings\n./funda sync --category rent --profile chrome\n\n# Search your local database\n./funda search --city utrecht --category rent`
};
const code = document.getElementById('install-code');
const copy = document.getElementById('copy-command');
let current = 'docker';
function escapeHTML(s) { return s.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;'); }
function showSnippet(tab) {
  current = tab;
  code.innerHTML = snippets[tab].split('\n').map(line => line.startsWith('#') ? `<span class="code-comment">${escapeHTML(line)}</span>` : escapeHTML(line)).join('\n');
  document.querySelectorAll('.code-tab').forEach(btn => {
    const active = btn.dataset.tab === tab;
    btn.classList.toggle('active', active);
    btn.setAttribute('aria-selected', String(active));
  });
  copy.innerHTML = 'Copy <span>⧉</span>';
}
document.querySelectorAll('.code-tab').forEach(btn => btn.addEventListener('click', () => showSnippet(btn.dataset.tab)));
copy.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(snippets[current]);
    copy.textContent = 'Copied ✓';
  } catch {
    copy.textContent = 'Select text to copy';
  }
});
