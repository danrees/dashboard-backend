# Dashboard Backend  

Backend to supply data to a web based home dashboard.

Written in Go.

# Functions

## Weather

Provides weather data from [Open Weather Map](https://openweathermap.org/). 

## Agenda

Will sync up to a google calendar. Not implemented yet.

# Configuration

Env Vars:  
`WEATHER_API_KEY` - open weather map api key  
`WEATHER_CITY_ID` - city id to provide weather for, this can be found through a download on the open weather map website.

