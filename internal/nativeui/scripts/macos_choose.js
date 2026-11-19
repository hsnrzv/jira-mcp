function run() {
  const app = setupApp();
  const alert = newAlert();
  env('JMCP_BUTTONS').split('\n').filter(Boolean).forEach(t => alert.addButtonWithTitle(t));
  return String(runAlert(app, alert) - FIRST_BUTTON);
}
