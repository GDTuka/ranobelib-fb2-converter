# Как работает

Чтобы программа работала ей нужно передать конфиг параметр ввиде json
Пример json
```
{
    "bookName":"someName",
    "url":"https://ranobelib.me/ru/24263--about-the-reckless-girl-who-kept-challenging-a-reborn-man-like-me",
    "apiUrl":"https://api2.mangalib.me/api/manga/24263--about-the-reckless-girl-who-kept-challenging-a-reborn-man-like-me",
    "outputDir":"A:/work/ranobelib_parser/output"
}
```
url - ссылка на сам новеллу без доп query значений
apiUrl - этаже ссылка только на апи, всё что меняется это домен ranobelib.me/ru/ на api2.mangalib.me/api/manga/
outputDir - куда сохранится файл
bookName - имя файла

ссылка на ui проекта https://github.com/GDTuka/ranobelib-fb2-converter-ui