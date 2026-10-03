# JACoB Console Appearance Specification

> **GALNET TECHNICAL CIRCULAR // DISPLAY SYSTEMS**

Open **Settings → Appearance** to load an HTML appearance file.

The file may contain CSS for the JACoB host console and replacement templates for documented shell regions.

```html
<style>
:root {
  --accent: #ff8a00;
}

body {
  font-family: monospace;
}

.card {
  border-color: #ff8a00;
}
</style>

<template data-jacob-slot="brand">
  <strong>MY BRIDGE</strong><span>COMMAND CONSOLE</span>
</template>

<template data-jacob-slot="header-extra">...</template>
<template data-jacob-slot="nav-extra">...</template>
<template data-jacob-slot="home-extra">...</template>
<template data-jacob-slot="footer">...</template>
```

Optional body class:

```html
<meta name="jacob-body-class" content="my-theme">
```

JACoB stores the uploaded file locally and applies it to browsers connected to that instance.

Scripts contained in an appearance file are not executed in the host page. Interactive functions belong in sandboxed custom tabs.
