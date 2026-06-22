function errorMessage(error) {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  return 'request failed';
}

function normalizePathBrowserResult(result, requestedError = '') {
  return {
    path: result?.path || '',
    parent: result?.parent || '',
    directories: Array.isArray(result?.directories) ? result.directories : [],
    error: '',
    requestedError,
  };
}

function normalizePathBrowserError(error, requestedError = '') {
  return {
    path: '',
    parent: '',
    directories: [],
    error: errorMessage(error),
    requestedError,
  };
}

function buildEditableLibrary(library) {
  return {
    id: library.id ?? null,
    path: library.path || '',
    enabled: !!library.enabled,
    name: library.name || '',
  };
}

function buildEditableFederation(federation = {}) {
  return {
    enabled: !!federation.enabled,
    role: federation.role || '',
    hub_addr: federation.hub_addr || '',
    listen: federation.listen || '',
    token: federation.token || '',
    peer_id: federation.peer_id || '',
  };
}

function buildEditableScanSettings(scan = {}) {
  return {
    assume_same_title_folder_compilations: !!scan.assume_same_title_folder_compilations,
  };
}

export function buildEditableSettingsSnapshot({ signupEnabled, guestAccess, libraries = [], federation = {}, scan = {} }) {
  return JSON.stringify({
    signupEnabled: !!signupEnabled,
    guestAccess: !!guestAccess,
    libraries: libraries.map(buildEditableLibrary),
    federation: buildEditableFederation(federation),
    scan: buildEditableScanSettings(scan),
  });
}

export function hasEditableSettingsChanges(cleanSnapshot, state) {
  return buildEditableSettingsSnapshot(state) !== cleanSnapshot;
}

export function beforeUnloadIfDirty(isDirty, event) {
  if (!isDirty) return;

  event.preventDefault();
  event.returnValue = '';
}

export async function loadPathBrowserLocation(listLibraryPaths, path, allowFallback) {
  try {
    const location = path ? await listLibraryPaths(path) : await listLibraryPaths();
    return normalizePathBrowserResult(location);
  } catch (error) {
    const requestedError = errorMessage(error);
    if (!path || !allowFallback) return normalizePathBrowserError(error);

    try {
      const fallbackLocation = await listLibraryPaths();
      return normalizePathBrowserResult(fallbackLocation, requestedError);
    } catch (fallbackError) {
      return normalizePathBrowserError(fallbackError, requestedError);
    }
  }
}
