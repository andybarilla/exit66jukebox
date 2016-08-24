package com.exit66.jukebox.data;

import com.exit66.jukebox.util.MediaFile;

public class ScanRunnable implements Runnable {

	private String _fileName;
	private int _libraryID;
	private String _libraryPath;
	private boolean _scanAll;
	
	public ScanRunnable(String fileName, int libraryID, String libraryPath, boolean scanAll)
	{	
		_libraryID = libraryID;
		_libraryPath = libraryPath;
		_fileName = fileName;
		_scanAll = scanAll;
	}
	
	@Override
	public void run() {
		if (_scanAll == false) {
			Track t = new Track();

			t.setLibraryID(_libraryID);
			t.setFileName(_fileName);
			if (t.find()) {
				return;
			}
		}
		MediaFile m = new MediaFile(_fileName);
		m.setLibraryID(_libraryID);
		m.setLibraryPath(_libraryPath);
		if (m.importTagInfo() == false) {

			Library l = new Library();
			l.setLibraryID(_libraryID);
			l.addFailedFile(_fileName,
					_fileName.substring(_libraryPath.length()));

		}
	}

}
