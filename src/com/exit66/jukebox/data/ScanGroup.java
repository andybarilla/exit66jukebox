package com.exit66.jukebox.data;

// TODO synonyms on find - & = AND, 'n'
import java.io.File;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import com.exit66.jukebox.Options;

public class ScanGroup extends Thread {

	private String _fileSep;
	private int _libraryID;
	private String _libraryPath;
	private boolean _scanAll;
	private ExecutorService _executor;

	public ScanGroup(int libraryID, boolean scanAll) {

		Library l = new Library();
		l.setLibraryID(libraryID);
		l.fetch();
		_libraryPath = l.getLibraryPath();
		l.setScanStatus(1);
		l.save();

		System.out.println("Started scanning library (id=" + libraryID
				+ ") at " + Options.getCurrentTime());
		setName("Exit66 ScanGroup");
		_libraryID = libraryID;
		_scanAll = scanAll;
		_fileSep = System.getProperty("file.separator");
		this.start();

	}

	public void run() {

		_executor = Executors.newFixedThreadPool(Options.getScanThreadCount());
		
		recurse(new File(_libraryPath));
		
		// This will make the executor accept no new threads
		// and finish all existing threads in the queue
		_executor.shutdown();
		// Wait until all threads are finish
		while (!_executor.isTerminated()) {

		}
		
		Library l = new Library();
		l.setLibraryID(_libraryID);
		l.fetch();
		l.setScanStatus(0);
		l.save();
		System.out.println("Completed scanning library (id="
				+ _libraryID + ") at " + Options.getCurrentTime());
		Maintenance maint = new Maintenance();
		maint.cleanDatabase();
	}

	private void recurse(File dir) {

		String extension;

		if ((dir != null) && (dir.isDirectory())) {

			String dirlist[] = dir.list();

			for (int i = 0; i < dirlist.length; i++) {

				File importFile = new File(dir.getPath() + _fileSep
						+ dirlist[i]);
				if (importFile.isDirectory()) {

					recurse(importFile);

				} else {

					extension = Options.getExtension(importFile.getName())
							.toUpperCase();
					if ((extension.compareTo("MP3") == 0)
							|| (extension.compareTo("OGG") == 0))
						_executor.execute(new ScanRunnable(importFile.getAbsolutePath(), _libraryID, _libraryPath, _scanAll));

				}
			}
		}
	}
}
