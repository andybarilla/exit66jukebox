package com.exit66.jukebox.tag;

import java.io.ByteArrayOutputStream;

/**
 * @author andyb
 * 
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates. To enable and disable the creation of type
 * comments go to Window>Preferences>Java>Code Generation.
 */

public class TagReader {

	protected String _artistName = "";
	protected String _albumName = "";
	protected String _albumArtistName = "";	
	protected String _trackName = "";
	protected String _contentType = "";
	protected int _trackNumber = 0;
	protected String _fileName = "";
	protected String _coverMimeType = "";
	protected ByteArrayOutputStream _coverImage = null;
	protected boolean _hasTag = false;
	protected String _time = "";
	
	public TagReader(String fileName) {
		_hasTag = readTag(fileName);
	}
	
	public boolean hasTag() {
		return _hasTag;
	}
	
	public boolean readTag(String s) {

		_fileName = s;
		return readTag();

	}

	public boolean readTag() {

		return false;

	}

	public String getArtistName() {

		return _artistName;

	}

	public void setArtistName(String newValue) {

		_artistName = newValue;

	}

	public String getAlbumName() {

		return _albumName;

	}

	public void setAlbumName(String newValue) {

		_albumName = newValue;

	}

	public String getAlbumArtistName() {

		return _albumArtistName;

	}

	public void setAlbumArtistName(String newValue) {

		_albumArtistName = newValue;

	}

	public String getTrackName() {

		return _trackName;

	}

	public void setTrackName(String newValue) {

		_trackName = newValue;

	}

	public int getTrackNumber() {

		return _trackNumber;

	}

	public void setTrackNumber(int newValue) {

		_trackNumber = newValue;

	}

	public String getContentType() {

		return _contentType;

	}

	public void setContentType(String newValue) {

		_contentType = newValue;

	}

	public String getFileName() {

		return _fileName;

	}

	public void setFileName(String newValue) {

		_fileName = newValue;

	}
	
	public String getCoverMimeType() {
		if (_coverMimeType.compareTo("image/jpg") == 0) {
			return "image/jpeg";
		}
		return _coverMimeType;
	}		
	
	public byte[] getCoverImage() {
		return _coverImage.toByteArray();
	}
	
	public boolean loadCoverImage() {

		return false;

	}
	
	public String getTime() {

		return _time;

	}

	public void setTime(String newValue) {

		_time = newValue;

	}

	public void setTime(int newValue) {
		int hours;
		int minutes;
		int seconds;
		
		if (newValue == 0) {
			_time = "";
		}
		
		hours = newValue / 3600;
		newValue = newValue - hours * 3600;
		minutes = newValue / 60;
		newValue = newValue - minutes * 60;
		seconds = newValue;
		
		if (hours == 0) {
			_time = String.format("%02d:%02d", minutes, seconds);
		} else {
			_time = String.format("%02d:%02d:%02d", hours, minutes, seconds); 
		}
	}
	
	public int toInt(Number value) {
		if (value == null) {
			return 0;
		}
		else {
			return value.intValue();
		}
	}
}
 