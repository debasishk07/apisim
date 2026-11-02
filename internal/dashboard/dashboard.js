/* Dashboard JavaScript for apisim */
document.addEventListener('DOMContentLoaded', () => {
  // State
  let state = {
    running: false,
    paused: false,
    events: [],
    eventSource: null,
    filterText: '',
    filterStatus: 'all'
  };
  
  // Element references
  const homeScreen = document.getElementById('home-screen');
  const trafficScreen = document.getElementById('traffic-screen');
  const chaosScreen = document.getElementById('chaos-screen');
  const reportScreen = document.getElementById('report-screen');
  const recordingsScreen = document.getElementById('recordings-screen');
  
  const startBlankBtn = document.getElementById('start-blank');
  const backHomeBtn = document.getElementById('back-home');
  const pauseBtn = document.getElementById('pause-btn');
  const killSwitchBtn = document.getElementById('kill-switch');
  const filterQ = document.getElementById('filter-q');
  const filterStatus = document.getElementById('filter-status');
  const latencySlider = document.getElementById('latency-slider');
  const probabilitySlider = document.getElementById('probability-slider');
  const errorStatusSelect = document.getElementById('error-status');
  const rateLimitInput = document.getElementById('rate-limit');
  const rateWindowInput = document.getElementById('rate-window');
  const timeoutSlider = document.getElementById('timeout-slider');
  const resetBtn = document.getElementById('reset-btn');
  const seedDisplay = document.getElementById('seed-display');
  const reportFormat = document.getElementById('report-format');
  const exportBtn = document.getElementById('export-btn');
  
  // Initialize Alpine.js
  Alpine.start();
  
  // Screen transitions
  function showScreen(screen) {
    [homeScreen, trafficScreen, chaosScreen, reportScreen, recordingsScreen]
      .forEach(s => s.classList.add('hidden'));
    screen.classList.remove('hidden');
  }
  
  // Start blank scenario
  startBlankBtn.addEventListener('click', () => {
    // TODO: Start apisim with blank scenario
    alert('Starting blank scenario - feature coming soon');
  });
  
  // Navigate back home
  backHomeBtn.addEventListener('click', () => showScreen(homeScreen));
  document.getElementById('back-to-traffic').addEventListener('click', () => showScreen(trafficScreen));
  document.getElementById('back-to-traffic2').addEventListener('click', () => showScreen(trafficScreen));
  document.getElementById('back-to-traffic3').addEventListener('click', () => showScreen(trafficScreen));
  
  // Pause/resume
  pauseBtn.addEventListener('click', () => {
    state.paused = !state.paused;
    pauseBtn.textContent = state.paused ? 'Resume' : 'Pause';
  });
  
  // Kill switch
  killSwitchBtn.addEventListener('click', () => {
    const isOn = killSwitchBtn.textContent.includes('ON');
    killSwitchBtn.textContent = isOn ? 'Kill Switch: OFF' : 'Kill Switch: ON';
  });
  
  // Filter events
  filterQ.addEventListener('input', (e) => {
    state.filterText = e.target.value;
    filterEvents();
  });
  filterStatus.addEventListener('change', (e) => {
    state.filterStatus = e.target.value;
    filterEvents();
  });
  
  // Control handlers
  latencySlider.addEventListener('input', (e) => {
    // TODO: Send latency control to apisim
    console.log('Latency:', e.target.value, 'ms');
  });
  
  probabilitySlider.addEventListener('input', (e) => {
    // TODO: Send probability control to apisim
    console.log('Probability:', e.target.value, '%');
  });
  
  errorStatusSelect.addEventListener('change', (e) => {
    // TODO: Send error status control to apisim
    console.log('Error status:', e.target.value);
  });
  
  rateLimitInput.addEventListener('input', (e) => {
    // TODO: Send rate limit control to apisim
    console.log('Rate limit:', e.target.value);
  });
  
  rateWindowInput.addEventListener('input', (e) => {
    // TODO: Send rate window control to apisim
    console.log('Rate window:', e.target.value, 's');
  });
  
  timeoutSlider.addEventListener('input', (e) => {
    // TODO: Send timeout control to apisim
    console.log('Timeout probability:', e.target.value, '%');
  });
  
  resetBtn.addEventListener('click', () => {
    // TODO: Reset all controls
    latencySlider.value = 0;
    probabilitySlider.value = 30;
    errorStatusSelect.value = '503';
    rateLimitInput.value = '10';
    rateWindowInput.value = '60';
    timeoutSlider.value = 0;
  });
  
  // Report export
  exportBtn.addEventListener('click', () => {
    const format = reportFormat.value;
    // TODO: Export report
    alert(`Exporting report as ${format}`);
  });
  
  // Helper functions
  function filterEvents() {
    const eventsDiv = document.getElementById('events-container');
    // TODO: Filter and display events
  }
});