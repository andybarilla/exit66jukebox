package com.exit66.jukebox.tag;

import java.io.File;

import com.jcraft.jorbis.*;

/**
 * This class gives information (audio format and comments) about Ogg Vorbis file.
 */
public class OggReader extends TagReader {
	
	public OggReader(String fileName) {
		super(fileName);
	}

	public boolean readTag() {
		try {
			File file = new File(_fileName);
			if (!file.exists()) 
				return false;
			VorbisFile vorbisfile = new VorbisFile(_fileName);
			for (int i=0; i < vorbisfile.streams(); i++) {
				Comment comments = vorbisfile.getComment(i);
				for (int j=0; j < comments.comments; j++) {
					String comment = comments.getComment(j);
					int pos = comment.indexOf("=");
					if (pos != -1) {
						String key = comment.substring(0, pos).trim();
						String val = comment.substring(pos+1).trim();
						if (key.equalsIgnoreCase("artist"))
							_artistName = val;
						else if (key.equalsIgnoreCase("album"))
							_albumName = val;
						else if (key.equalsIgnoreCase("title"))
							_trackName = val;
						else if (key.equalsIgnoreCase("tracknumber") || key.equalsIgnoreCase("track")) {
							String[] track_num = val.split("/", 2);
							_trackNumber = Integer.parseInt(track_num[0]);
						}
						else if (key.equalsIgnoreCase("genre"))
							_contentType = val;
					}
				}
			}
			if (vorbisfile.streams() > 0)
				return true;
			else 
				return false;
		}
		catch (Exception e) {
			System.err.println(e);
			return false;
		}
	}
}