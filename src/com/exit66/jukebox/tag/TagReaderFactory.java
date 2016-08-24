package com.exit66.jukebox.tag;

import com.exit66.jukebox.Options;

public class TagReaderFactory {

	public static TagReader getTagReader(String fileName) {
		String ext = Options.getExtension(fileName);
		if (ext.equals("ogg")) {
			return new OggReader(fileName);
		}
		else if (ext.equals("mp3")) {
			return new ID3Reader(fileName);
		}
		return null;
	}

}
