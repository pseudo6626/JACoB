# JACoB UI themes

Open **Settings → Appearance** and upload an HTML theme file.

A theme can override the entire visual presentation of the JACoB host using ordinary CSS. It can also replace documented shell regions.

Language is configured separately under **Settings → Language**. Appearance HTML controls styling and documented shell slots; it does not translate arbitrary tab content. Locale-aware custom tabs should use SDK v6 `Elite.locale`.

```html
<style>
:root {
  --accent: #7fd5ff;
}

body {
  font-family: monospace;
}

.card {
  border-color: #7fd5ff;
}
</style>

<template data-jacob-slot="brand">
  <strong>MY BRIDGE</strong><span>Elite tools</span>
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

The uploaded HTML is persisted by JACoB and applied to browsers connected to that JACoB instance.

Theme scripts are not executed in the privileged host page. Interactive tools belong in custom tabs.
