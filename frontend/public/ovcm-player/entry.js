if (new URLSearchParams(location.search).get('local') === '1') {
  await import('./local-course.js');
} else {
  await import('./assets/index-BoyDnFJg.js');
}
