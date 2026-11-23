const picker = document.querySelector('.language-picker');

if (picker) {
  document.addEventListener('click', (event) => {
    if (event.target instanceof Node && !picker.contains(event.target)) picker.open = false;
  });
  picker.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape' || !picker.open) return;
    picker.open = false;
    picker.querySelector('summary')?.focus();
  });
}
